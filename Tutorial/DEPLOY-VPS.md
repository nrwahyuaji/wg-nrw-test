# Deploy WG-WebUI ke VPS (Tahap 20, UAT)

Panduan ini untuk VPS baru (Ubuntu 22.04/24.04 amd64) dengan akses root/sudo.
Repo publik: https://github.com/nrwahyuaji/wg-nrw-test

## 0. Prasyarat VPS
- Kernel WireGuard tersedia: `modinfo wireguard` (kernel >= 5.6) atau modul terpasang. VPS OpenVZ/LXC biasanya TIDAK punya modul → gagal di step 0.
- Port panel 8091 dan UDP 51820 terbuka di firewall VPS (Security Group / ufw).
- Domain (opsional) diarahkan ke VPS via reverse proxy TLS (Nginx Proxy Manager).

## 1. Clone repo
```bash
cd /opt
git clone https://github.com/nrwahyuaji/wg-nrw-test.git
cd wg-nrw-test
```

## 2. Jalankan installer
```bash
sudo bash WG-Server/deploy/install.sh \
  /var/lib/wgpanel \
  /var/lib/wgpanel/backups \
  /etc/wireguard \
  127.0.0.1:8091 \
  51820
```
Argumen: `<db_dir> <backup_dir> <wg_conf_dir> <listen> <udp_port>`.

Installer otomatis: cek modul wireguard, ip_forward=1, user wgpanel, folder DB/backup/wgconf, biner, unit systemd, `/etc/wgpanel.env` (mode 600), WGPANEL_SECRET_KEY (generate bila tidak ada), restart unit.

## 3. Post-install (manual, sysadmin)
```bash
# NAT + ip_forward permanen (jika belum)
sudo sysctl -w net.ipv4.ip_forward=1
echo 'net.ipv4.ip_forward=1' | sudo tee /etc/sysctl.d/99-wgpanel.conf
sudo sysctl --system

# Firewall UDP WireGuard
sudo ufw allow 51820/udp
sudo ufw allow 8091/tcp   # hanya bila panel langsung (tanpa reverse proxy)

# Buat admin
sudo wgpanel create-admin -username admin

# Apply kernel dari DB
sudo systemctl start wgpanel-apply
sudo systemctl status wgpanel-apply --no-pager
```

## 4. Verifikasi
```bash
sudo systemctl status wgpanel --no-pager
sudo systemctl status wgpanel-dev --no-pager 2>/dev/null || true
curl -s http://127.0.0.1:8091/healthz
curl -s http://127.0.0.1:8091/readyz
sudo wgpanel version
```
Buka panel: `http://<ip-vps>:8091` (atau domain reverse proxy).

## 5. Reverse proxy TLS (Nginx Proxy Manager / Nginx)
- Arahkan domain panel ke `127.0.0.1:8091`.
- Aktifkan WebSocket (untuk status live).
- Set `WGPANEL_ALLOWED_ORIGINS` di `/etc/wgpanel.env` bila perlu.

## 6. Troubleshooting
- `modinfo wireguard` gagal → VPS tidak support kernel module. Ganti VPS (KVM, bukan OpenVZ/LXC).
- `bind() to ...:8091 failed (99)` → port dipakai; ubah `WGPANEL_LISTEN` di env.
- Kernel menolak spec → cek `journalctl -u wgpanel -e`.
- DB migration gagal → cek `journalctl -u wgpanel-apply -e`.

## 7. Update (pull + rebuild)
```bash
cd /opt/wg-nrw-test
git pull
sudo bash WG-Server/deploy/install.sh /var/lib/wgpanel /var/lib/wgpanel/backups /etc/wireguard 127.0.0.1:8091 51820
sudo systemctl restart wgpanel
```
Tag `fase-12` = versi stabil terakhir (commit 3189e78). Untuk update ke versi terbaru, gunakan `main`.
