#!/usr/bin/env bash
# WG-WebUI INSTALL di VPS baru. Template - jangan commit nilai riil.
#   sudo bash install.sh [DB_PATH] [BACKUP_DIR] [WG_CONF_DIR] [LISTEN] [UDP_PORT]
# Sisa tugas teritorial sysadmin (bukan install):
#   - NAT masquerade subnet tunnel via <uplink> (iptables/nftables)
#   - buka UDP <UDP_PORT> di firewall
#   - wgpanel doctor; create-admin; import-conf opsional
#   - backup rsync/rclone ke luar VPS
# Prasyarat: biner wgpanel sudah copy ke /usr/local/bin/wgpanel
set -euo pipefail

# Lokasi script ini, biar install.sh bisa dijalankan dari path apapun.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

DB_PATH="${1:-/var/lib/wgpanel/wgpanel.db}"
BACKUP_DIR="${2:-/var/lib/wgpanel/backups}"
WG_CONF_DIR="${3:-/etc/wireguard}"
LISTEN="${4:-127.0.0.1:8091}"
UDP_PORT="${5:-51820}"

# Validasi argumen dasar sebelum memulai.
if ! [[ "${UDP_PORT}" =~ ^[0-9]+$ ]] || [ "${UDP_PORT}" -lt 1 ] || [ "${UDP_PORT}" -gt 65535 ]; then
  echo "ERROR: UDP_PORT '${UDP_PORT}' bukan port valid (1-65535)." >&2
  exit 1
fi
if ! [[ "${LISTEN}" =~ ^[^:]+:[0-9]+$ ]]; then
  echo "ERROR: LISTEN '${LISTEN}' format salah. Contoh: 127.0.0.1:8091" >&2
  exit 1
fi
if [ ! -f /usr/local/bin/wgpanel ]; then
  echo "ERROR: Biner '/usr/local/bin/wgpanel' belum ada." >&2
  echo "ERROR: Build dulu pakai 'make build' (dev/repo), lalu salin ke VPS." >&2
  exit 1
fi

# 0. Prasyarat kernel / jaringan (cek modul + net.ipv4.ip_forward)
#     Cuma verifikasi di sini; panel TIDAK mengubah kernel. Gagal di sini = hentikan.
echo ">>> [0/4] Prasyarat kernel & jaringan"

if ! command -v modinfo >/dev/null 2>&1; then
  echo "ERROR: perintah 'modinfo' tidak ada. Pasang paket 'kmod' (Debian/Ubuntu)."
  exit 1
fi
if ! modinfo wireguard >/dev/null 2>&1; then
  echo "ERROR: modul kernel 'wireguard' tidak ditemukan."
  echo "      Cek kernel >= 5.6 + linux-modules-extra, bukan 'apt install wireguard'."
  exit 1
fi
echo ">>> Modul WireGuard ditemukan: $(modinfo -F name wireguard) $(modinfo -F version wireguard 2>/dev/null || echo '?')"

if ! grep -q '^net.ipv4.ip_forward' /etc/sysctl.d/99-wireguard.conf 2>/dev/null; then
  echo 'net.ipv4.ip_forward=1' > /etc/sysctl.d/99-wireguard.conf
fi
sysctl -q -p /etc/sysctl.d/99-wireguard.conf
echo ">>> net.ipv4.ip_forward = $(sysctl -q -n net.ipv4.ip_forward 2>/dev/null || echo 'tidak terbaca')"

# 0b. Firewall UDP (default WireGuard: ${UDP_PORT:-51820})
if ! command -v ufw >/dev/null 2>&1; then
  export DEBIAN_FRONTEND=noninteractive
  echo ">>> ufw tidak ditemukan, menginstall..."
  apt-get update -y
  apt-get install -y ufw
fi

ufw allow ${UDP_PORT}/udp
ufw allow 22/tcp          # cegah lockout SSH saat ufw diaktifkan
ufw --force enable
echo ">>> Firewall UDP terbuka: ${UDP_PORT}/udp; ufw diaktifkan (SSH 22 dicegah lockout)."

# 0c. NAT masquerade subnet tunnel via uplink (opsional, iptables/nftables)
#     Isi WGPANEL_UPLINK dan WGPANEL_SUBNET di sini, atau lewati:
#       WGPANEL_UPLINK=eth0 WGPANEL_SUBNET=10.8.0.0/24 sudo bash deploy/install.sh ...
: "${WGPANEL_UPLINK:=${UPLINK:-}}"
: "${WGPANEL_SUBNET:=${SUBNET:-}}"
UPLINK="${WGPANEL_UPLINK:-${UPLINK:-}}"
SUBNET="${WGPANEL_SUBNET:-${SUBNET:-}}"

if [ -z "$UPLINK" ]; then
  if ip -o link show eth0 >/dev/null 2>&1; then
    UPLINK="eth0"
  elif ip -o link show enp0s3 >/dev/null 2>&1; then
    UPLINK="enp0s3"
  else
    echo ">>> WARN: UPLINK tidak diketahui. Skip NAT masquerade (set WGPANEL_UPLINK)."
    UPLINK=""
  fi
fi

if [ -z "$SUBNET" ]; then
  echo ">>> INFO: SUBNET belum diisi. NAT masquerade dilewati (set WGPANEL_SUBNET)."
  UPLINK=""
fi

if [ -n "$UPLINK" ]; then
  echo ">>> NAT masquerade subnet $SUBNET via uplink $UPLINK"
  if command -v iptables >/dev/null 2>&1; then
    if ! iptables -t nat -C POSTROUTING -s "$SUBNET" -o "$UPLINK" -j MASQUERADE 2>/dev/null; then
      iptables -t nat -A POSTROUTING -s "$SUBNET" -o "$UPLINK" -j MASQUERADE
      echo ">>> iptables rule: nat POSTROUTING src $SUBNET oif $UPLINK masquerade"
    else
      echo ">>> iptables rule sudah ada."
    fi
  elif command -v nft >/dev/null 2>&1; then
    nft add rule ip nat postrouting ip saddr "$SUBNET" oifname "$UPLINK" masquerade
    echo ">>> nft rule: masquerade src $SUBNET via $UPLINK"
  else
    echo ">>> WARN: iptables/nft tidak ditemukan. NAT masquerade tidak dikonfigurasi."
  fi
else
  echo ">>> INFO: NAT masquerade dilewati (UPLINK kosong)."
fi



# 1. user + folder
id -u wgpanel >/dev/null 2>&1 || useradd -r -s /usr/sbin/nologin -d /var/lib/wgpanel wgpanel
install -d -o wgpanel -g wgpanel -m 700 /var/lib/wgpanel /var/lib/wgpanel/backups "$WG_CONF_DIR"

# 2. binary + unit produksi
install -m 755 /usr/local/bin/wgpanel /usr/local/bin/wgpanel
install -m 644 "$SCRIPT_DIR/wgpanel.service" "$SCRIPT_DIR/wgpanel-apply.service" /etc/systemd/system/
systemctl daemon-reload
systemctl enable wgpanel-apply wgpanel
systemctl restart wgpanel-apply   # oneshot: siapkan DB + migrate skema

# 3. env (template example tanpa rahasia) + isi yang bisa di-set CLI
install -m 640 -o root -g wgpanel wgpanel.env.example /etc/wgpanel.env
# Hapus baris WGPANEL_SECRET_KEY lama (env.example kosong) supaya tidak ter-duplicate
sed -i "/^WGPANEL_SECRET_KEY=/d" /etc/wgpanel.env
sed -i \
    -e "s|^WGPANEL_DB_PATH=.*|WGPANEL_DB_PATH=$DB_PATH|" \
    -e "s|^WGPANEL_BACKUP_DIR=.*|WGPANEL_BACKUP_DIR=$BACKUP_DIR|" \
    -e "s|^WGPANEL_LISTEN=.*|WGPANEL_LISTEN=$LISTEN|" \
    -e "s|^WGPANEL_WG_CONF_DIR=.*|WGPANEL_WG_CONF_DIR=$WG_CONF_DIR|" \
    -e "s|^WGPANEL_TRUSTED_PROXIES=.*|WGPANEL_TRUSTED_PROXIES=127.0.0.1/32|" \
    /etc/wgpanel.env

# WGPANEL_SECRET_KEY: gunakan variable env kalau ada, kalau tidak generate baru
if [ -n "${WGPANEL_SECRET_KEY:-}" ]; then
  printf '%s\n' "WGPANEL_SECRET_KEY=$WGPANEL_SECRET_KEY" >> /etc/wgpanel.env
  echo "INFO: WGPANEL_SECRET_KEY telah digunakan dari environment."
else
  printf '%s\n' "WGPANEL_SECRET_KEY=$(openssl rand -base64 32)" >> /etc/wgpanel.env
  echo "INFO: WGPANEL_SECRET_KEY baru di-generate (openssl rand -base64 32). SIMPAN salin offline: DEPLOY.md §7."
fi
chown root:wgpanel /etc/wgpanel.env
chmod 600 /etc/wgpanel.env

# 4. ringkasan akhir (status service)
echo "SELESAI."
echo "Services: wgpanel-apply=$(systemctl is-active wgpanel-apply) | wgpanel=$(systemctl is-active wgpanel)."
echo "Kostum teritorial: NAT ke subnet tunnel (<uplink>), buka UDP $UDP_PORT di firewall,"
echo "lalu jalankan: wgpanel doctor -> create-admin -> (opsional) import-conf."
