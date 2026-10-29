// Package reconcile menyamakan kernel WireGuard dengan isi basis data (DESIGN 5.2). Semua perubahan yang
// harus terlihat di kernel lewat sini, sehingga hanya ada satu jalur tulis ke internal/wg (AGENTS.md §2).
package reconcile

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"

	"wgpanel/internal/secret"
	"wgpanel/internal/sqlc"
	"wgpanel/internal/wg"
	"wgpanel/internal/wgconf"
)

// Kernel adalah bagian Engine yang dipakai Reconciler. Interface ini ada supaya uji transaksi bisa memakai
// engine palsu tanpa root; produksi selalu memakai *wg.Engine.
type Kernel interface {
	Apply(spec wg.InterfaceSpec) error
	Remove(name string) error
}

// Tx diberikan ke fn di WithApply: Queries untuk query bertipe, Raw untuk SQL langsung pada transaksi yang sama.
type Tx struct {
	Queries *sqlc.Queries
	Raw     *sql.Tx
}

// Reconciler memegang satu mutex global yang menserialkan setiap penerapan ke kernel. Tanpa mutex, dua request
// bersamaan bisa membuat kernel menerima spec lama sesudah spec baru.
type Reconciler struct {
	mu      sync.Mutex
	db      *sql.DB
	kernel  Kernel
	box     *secret.Box
	confDir string // "" = jangan tulis salinan wg-quick
}

// New membuat Reconciler. confDir kosong mematikan penulisan /etc/wireguard/<nama>.conf (dipakai di dev).
func New(database *sql.DB, kernel Kernel, box *secret.Box, confDir string) *Reconciler {
	return &Reconciler{db: database, kernel: kernel, box: box, confDir: confDir}
}

// WithApply menjalankan fn di dalam transaksi lalu menerapkan interface id ke kernel SEBELUM commit.
// Urutan ini disengaja: kalau kernel menolak spec baru, transaksi di-rollback dan kernel diterapkan ulang dari
// DB yang belum berubah, sehingga DB dan kernel tidak pernah berbeda. Galat yang dikembalikan selalu galat
// asli (fn atau Apply), bukan galat pemulihan; galat pemulihan hanya dicatat ke log.
// id = 0 berarti fn sendiri yang menentukan interface (mis. membuat interface baru): fn mengembalikannya.
// fn menerima Tx (bukan hanya Queries) karena beberapa penulisan tidak punya query sqlc.
func (reconciler *Reconciler) WithApply(ctx context.Context, id int64, fn func(tx Tx) (int64, error)) error {
	reconciler.mu.Lock()
	defer reconciler.mu.Unlock()

	tx, err := reconciler.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	queries := sqlc.New(tx)
	target, err := fn(Tx{Queries: queries, Raw: tx})
	if err == nil && target == 0 {
		target = id
	}
	if err == nil {
		err = reconciler.applyLocked(ctx, queries, target)
	}
	if err != nil {
		_ = tx.Rollback() // galat asli di bawah lebih berguna daripada galat rollback
		reconciler.restoreLocked(ctx, target)
		return err
	}
	if err := tx.Commit(); err != nil {
		// Jarang (mis. disk penuh): kernel sudah memakai spec baru tetapi DB tidak. Kembalikan kernel ke DB.
		reconciler.restoreLocked(ctx, target)
		return fmt.Errorf("reconcile: commit: %w", err)
	}
	return nil
}

// ApplyAll menerapkan semua interface: enabled=1 → Apply, enabled=0 → Remove. Dipanggil `wgpanel apply`
// saat boot dan `serve` saat start. Satu interface gagal tidak menghentikan yang lain; galatnya digabung.
// Return jumlah interface yang aktif di kernel.
func (reconciler *Reconciler) ApplyAll(ctx context.Context) (int, error) {
	reconciler.mu.Lock()
	defer reconciler.mu.Unlock()
	queries := sqlc.New(reconciler.db)
	interfaces, err := queries.ListInterfaces(ctx)
	if err != nil {
		return 0, err
	}
	applied := 0
	var failures []error
	for _, iface := range interfaces {
		if iface.Enabled == 0 {
			if err := reconciler.kernel.Remove(iface.Name); err != nil {
				failures = append(failures, fmt.Errorf("%s: %w", iface.Name, err))
			}
			continue
		}
		if err := reconciler.applyLocked(ctx, queries, iface.ID); err != nil {
			failures = append(failures, fmt.Errorf("%s: %w", iface.Name, err))
			continue
		}
		applied++
	}
	return applied, errors.Join(failures...)
}

// restoreLocked menerapkan ulang interface dari DB di luar transaksi setelah kegagalan. Interface yang belum
// ada di DB (POST yang gagal) tidak punya keadaan lama, jadi link setengah jadi milik nama itu dihapus.
func (reconciler *Reconciler) restoreLocked(ctx context.Context, id int64) {
	if id == 0 {
		return
	}
	queries := sqlc.New(reconciler.db)
	if _, err := queries.GetInterface(ctx, id); errors.Is(err, sql.ErrNoRows) {
		return
	}
	if err := reconciler.applyLocked(ctx, queries, id); err != nil {
		slog.Error("reconcile: gagal memulihkan kernel dari DB", "interface_id", id, "err", err)
	}
}

// applyLocked membaca interface + peer aktif dari queries (bisa transaksi), membangun spec, menerapkannya, lalu
// menulis salinan wg-quick. Pemanggil wajib memegang mutex.
func (reconciler *Reconciler) applyLocked(ctx context.Context, queries *sqlc.Queries, id int64) error {
	iface, err := queries.GetInterface(ctx, id)
	if err != nil {
		return fmt.Errorf("reconcile: interface %d: %w", id, err)
	}
	if iface.Enabled == 0 {
		return reconciler.kernel.Remove(iface.Name)
	}
	peers, err := queries.ListActivePeersByInterface(ctx, id)
	if err != nil {
		return err
	}
	spec, err := reconciler.buildSpec(iface, peers)
	if err != nil {
		return err
	}
	if err := reconciler.kernel.Apply(spec); err != nil {
		return err
	}
	if reconciler.confDir == "" {
		return nil
	}
	return writeWGQuick(reconciler.confDir, iface, spec)
}

// buildSpec mengubah baris DB menjadi wg.InterfaceSpec: dekripsi private key server dan PSK, lalu AllowedIPs
// sisi server = alamat peer (/32) + site_subnets (DESIGN 5.2 langkah 3).
func (reconciler *Reconciler) buildSpec(iface sqlc.Interface, peers []sqlc.Peer) (wg.InterfaceSpec, error) {
	address, err := netip.ParsePrefix(iface.Address)
	if err != nil {
		return wg.InterfaceSpec{}, fmt.Errorf("reconcile: alamat %s rusak: %w", iface.Name, err)
	}
	privateKey, err := reconciler.openKey(iface.PrivateKey)
	if err != nil {
		return wg.InterfaceSpec{}, fmt.Errorf("reconcile: private key %s: %w", iface.Name, err)
	}
	spec := wg.InterfaceSpec{
		Name: iface.Name, Address: address, ListenPort: int(iface.ListenPort), MTU: int(iface.Mtu),
		PrivateKey: privateKey,
	}
	for _, peer := range peers {
		peerSpec, err := reconciler.buildPeer(peer)
		if err != nil {
			return wg.InterfaceSpec{}, fmt.Errorf("reconcile: peer %d: %w", peer.ID, err)
		}
		spec.Peers = append(spec.Peers, peerSpec)
	}
	return spec, nil
}

// buildPeer menyusun sisi server satu peer. PSK NULL = tanpa PSK.
func (reconciler *Reconciler) buildPeer(peer sqlc.Peer) (wg.PeerSpec, error) {
	publicKey, err := wgtypes.ParseKey(peer.PublicKey)
	if err != nil {
		return wg.PeerSpec{}, fmt.Errorf("public key: %w", err)
	}
	allowed, err := parsePrefixes(peer.Address + "," + peer.SiteSubnets)
	if err != nil {
		return wg.PeerSpec{}, err
	}
	spec := wg.PeerSpec{PublicKey: publicKey, AllowedIPs: allowed}
	if len(peer.PresharedKey) > 0 {
		psk, err := reconciler.openKey(peer.PresharedKey)
		if err != nil {
			return wg.PeerSpec{}, fmt.Errorf("preshared key: %w", err)
		}
		spec.PresharedKey = &psk
	}
	return spec, nil
}

// openKey membuka blob terenkripsi berisi kunci WireGuard base64.
func (reconciler *Reconciler) openKey(blob []byte) (wgtypes.Key, error) {
	plain, err := reconciler.box.Open(blob)
	if err != nil {
		return wgtypes.Key{}, err
	}
	return wgtypes.ParseKey(string(plain))
}

// parsePrefixes membaca daftar CIDR dipisah koma; item kosong dilewati.
func parsePrefixes(raw string) ([]netip.Prefix, error) {
	var result []netip.Prefix
	for _, item := range strings.Split(raw, ",") {
		if item = strings.TrimSpace(item); item == "" {
			continue
		}
		prefix, err := netip.ParsePrefix(item)
		if err != nil {
			return nil, fmt.Errorf("CIDR %q rusak: %w", item, err)
		}
		result = append(result, prefix)
	}
	return result, nil
}

// writeWGQuick menulis salinan wg-quick server secara atomik: berkas sementara di direktori yang sama
// (agar rename tidak lintas filesystem) → 0600 → rename. Pembaca tidak pernah melihat berkas setengah jadi.
// Salinan ditulis SESUDAH kernel menerima spec, sehingga berkas tidak pernah berisi keadaan yang ditolak kernel.
func writeWGQuick(dir string, iface sqlc.Interface, spec wg.InterfaceSpec) error {
	temp, err := os.CreateTemp(dir, "."+iface.Name+".conf.*")
	if err != nil {
		return fmt.Errorf("reconcile: salinan wg-quick: %w", err)
	}
	tempName := temp.Name()
	defer func() { _ = os.Remove(tempName) }() // tidak berefek setelah rename berhasil
	if err := temp.Chmod(0o600); err != nil {
		_ = temp.Close()
		return err
	}
	if _, err := temp.WriteString(wgconf.RenderServer(serverConfig(iface, spec))); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(tempName, filepath.Join(dir, iface.Name+".conf"))
}

// serverConfig memetakan baris interface + spec yang baru diterapkan ke struktur render wgconf.
func serverConfig(iface sqlc.Interface, spec wg.InterfaceSpec) wgconf.ServerConfig {
	config := wgconf.ServerConfig{
		PrivateKey: spec.PrivateKey.String(), Address: iface.Address, ListenPort: spec.ListenPort, MTU: spec.MTU,
	}
	for _, peer := range spec.Peers {
		entry := wgconf.ServerPeer{PublicKey: peer.PublicKey.String()}
		if peer.PresharedKey != nil {
			entry.PresharedKey = peer.PresharedKey.String()
		}
		for _, prefix := range peer.AllowedIPs {
			entry.AllowedIPs = append(entry.AllowedIPs, prefix.String())
		}
		config.Peers = append(config.Peers, entry)
	}
	return config
}
