# Instalasi degoploy

Panduan memasang degoploy di server Linux sebagai service systemd, di belakang reverse proxy HTTPS.

## 1. Kebutuhan

| | |
|---|---|
| OS | Linux amd64 atau arm64 dengan systemd (Ubuntu 22.04+/Debian 12+ sudah diuji pola ini) |
| Paket | `git`, `bash`, `openssh-client` (untuk repo via SSH) |
| Jaringan | Reverse proxy HTTPS (Caddy/Nginx) yang bisa diakses GitHub/Gitea |
| Port internal | Bawaan `127.0.0.1:8090` (bisa diubah) |

degoploy adalah satu binary statis (UI sudah ter-embed); tidak perlu Go/Node di server.

## 2. Instal dari rilis

```bash
# ganti versi/arsitektur sesuai file rilis
tar xzf degoploy-<versi>-linux-amd64.tar.gz
cd degoploy-<versi>-linux-amd64
sha256sum -c ../SHA256SUMS --ignore-missing      # opsional, bila SHA256SUMS ikut diunduh
sudo ./install.sh --base-url https://deploy.example.com --admin admin
```

`install.sh` akan:

1. membuat user sistem `degoploy` (tanpa login shell),
2. memasang binary ke `/usr/local/bin/degoploy`,
3. menulis `/etc/degoploy/degoploy.yaml` dan `/etc/degoploy/degoploy.env` **jika belum ada**,
4. menjalankan `degoploy config check`,
5. dengan `--admin NAMA`: membuat admin lokal (password ditanyakan),
6. memasang `degoploy.service`, lalu `enable` + `start`.

Opsi lain: `--listen 127.0.0.1:9000`, `--no-start`, `--binary PATH`.

### Build sendiri

```bash
make dist            # release/degoploy-<versi>-linux-{amd64,arm64}.tar.gz + SHA256SUMS
```

## 3. Lokasi file

| Path | Isi | Izin |
|---|---|---|
| `/usr/local/bin/degoploy` | binary | `0755` root |
| `/etc/degoploy/degoploy.yaml` | konfigurasi | `0640` root:degoploy |
| `/etc/degoploy/degoploy.env` | secret (`DEGOPLOY_OIDC_SECRET=…`) | `0640` root:degoploy |
| `/var/lib/degoploy/` | database, deploy key, script, log | `0700` degoploy |
| `/etc/systemd/system/degoploy.service` | unit systemd | `0644` |

Ubah konfigurasi lalu cek dan restart:

```bash
sudo -u degoploy degoploy config check --config /etc/degoploy/degoploy.yaml
sudo systemctl restart degoploy
```

Perintah `degoploy` yang menyentuh database (`user …`, `backup`) jalankan sebagai user `degoploy`
(`sudo -u degoploy …`), agar file yang dibuat tetap milik service.

## 4. Reverse proxy

Set `base_url` ke URL publik (https). Proxy harus meneruskan WebSocket di `/ws` dan header `X-Forwarded-For`.
Proxy di host yang sama sudah dipercaya lewat `trusted_proxies: [127.0.0.1/32, "::1/128"]`.

**Caddy**

```caddy
deploy.example.com {
	reverse_proxy 127.0.0.1:8090
}
```

**Nginx**

```nginx
server {
    listen 443 ssl http2;
    server_name deploy.example.com;
    # ssl_certificate / ssl_certificate_key …

    client_max_body_size 10m;          # payload webhook

    location / {
        proxy_pass http://127.0.0.1:8090;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header Upgrade $http_upgrade;     # WebSocket /ws (live log)
        proxy_set_header Connection $connection_upgrade;
        proxy_read_timeout 1h;                      # koneksi realtime yang lama
    }
}

# di blok http {}
map $http_upgrade $connection_upgrade { default upgrade; '' close; }
```

Jika indikator di pojok kanan atas UI tetap "Reconnecting…", WebSocket belum diteruskan proxy.

## 5. Project pertama

1. **Folder kerja.** degoploy meng-clone ke `work_dir` sebagai user `degoploy`, jadi user itu harus bisa menulis di sana:

   ```bash
   sudo install -d -o degoploy -g www-data -m 2775 /var/www/my-app
   # atau untuk folder yang sudah ada: sudo setfacl -R -m u:degoploy:rwX -m d:u:degoploy:rwX /var/www/my-app
   ```

   Folder harus kosong atau sudah berupa repo git; folder berisi file lain ditolak agar tidak tertimpa.

2. **Buat project** di UI (*Projects → New project*), isi URL SSH repo dan *Branch to deploy*.
3. **Deploy key**: salin public key di *Overview → Deploy key* ke repo
   (GitHub: *Settings → Deploy keys*, Gitea: *Settings → Deploy Keys*), cukup *read-only*. Klik **Test connection**.
4. **Webhook**: salin *Payload URL* dan *Secret* dari *Overview → Webhook*.
   - GitHub: *Settings → Webhooks → Add webhook*, content type `application/json`, event *Just the push event*.
   - Gitea: *Settings → Webhooks → Add Webhook → Gitea*, method `POST`, content type `application/json`, *Push events*.

   Riwayat penerimaan muncul di *Webhook deliveries*; "Test delivery"/ping tercatat sebagai *ping*.
5. **Script deploy** di tab *Script* (atau file di repo). Contoh:

   ```bash
   set -euo pipefail
   composer install --no-dev --optimize-autoloader
   php artisan migrate --force
   sudo systemctl reload php8.3-fpm
   ```

Host key server Git disimpan otomatis saat koneksi pertama (`accept-new`) di `/var/lib/degoploy/known_hosts`.
Untuk menyematkannya lebih dulu:

```bash
ssh-keyscan github.com gitea.example.com | sudo -u degoploy tee -a /var/lib/degoploy/known_hosts
```

## 6. Hak akses script deploy

Script berjalan sebagai user `degoploy`. Untuk perintah yang butuh root, beri izin `sudo` spesifik
(jangan `ALL`):

```bash
sudo visudo -f /etc/sudoers.d/degoploy
```

```sudoers
degoploy ALL=(root) NOPASSWD: /usr/bin/systemctl restart my-app, /usr/bin/systemctl reload nginx
```

Catatan:
- Jalankan aplikasi sebagai service systemd dan restart lewat `systemctl` dari script. Proses yang
  dijalankan di background langsung dari script (`nohup …&`) ikut berhenti saat degoploy di-restart.
- Memasukkan `degoploy` ke grup `docker` setara dengan akses root.
- Unit sengaja tidak memakai `NoNewPrivileges`/`ProtectSystem` agar `sudo` dan penulisan ke `/var/www`
  tetap bisa; aktifkan lewat drop-in (`systemctl edit degoploy`) jika script Anda tidak membutuhkannya.

## 7. SSO (Casdoor)

Lihat bagian *SSO dengan Casdoor* di README. Secret masuk ke `/etc/degoploy/degoploy.env`:

```bash
echo 'DEGOPLOY_OIDC_SECRET=…' | sudo tee -a /etc/degoploy/degoploy.env
sudo systemctl restart degoploy
```

## 8. Upgrade

Unduh rilis baru lalu jalankan installer lagi:

```bash
sudo ./install.sh
```

Konfigurasi dan data tidak disentuh; migrasi database berjalan otomatis saat start. Deploy yang sedang
berjalan diberi waktu 60 detik untuk selesai, antrian dilanjutkan setelah restart.

## 9. Backup & restore

```bash
sudo install -d -o degoploy -g degoploy -m 0700 /var/backups/degoploy
sudo -u degoploy degoploy backup --config /etc/degoploy/degoploy.yaml \
  --output /var/backups/degoploy/backup-$(date +%F).tar.gz      # tambahkan --no-logs untuk lebih kecil
```

Arsip berisi snapshot database yang konsisten (aman saat service jalan), deploy key, script managed,
`known_hosts`, dan log. Simpan di luar server: arsip memuat private key.

Restore:

```bash
sudo systemctl stop degoploy
sudo -u degoploy tar xzf backup-2026-10-27.tar.gz -C /var/lib/degoploy
sudo systemctl start degoploy
```

## 10. Log & troubleshooting

```bash
journalctl -u degoploy -f
sudo -u degoploy degoploy config check --config /etc/degoploy/degoploy.yaml
```

| Gejala | Penyebab umum |
|---|---|
| Test connection: `Permission denied (publickey)` | Deploy key belum ditambahkan ke repo, atau URL repo bukan SSH |
| `Host key verification failed` | Host key server Git berubah; hapus barisnya di `/var/lib/degoploy/known_hosts` |
| Delivery `rejected: signature does not match` | Secret di webhook tidak sama dengan di degoploy (atau di-regenerate) |
| Delivery `404` | Payload URL salah atau project sudah dihapus |
| Deploy gagal `refusing to clone` | `work_dir` berisi file tetapi bukan repo git |
| Deploy gagal `mkdir … permission denied` | User `degoploy` tidak bisa menulis ke `work_dir` |
| Live log tidak muncul | Proxy tidak meneruskan WebSocket `/ws` |
| Login SSO: "not in a group" | Nama group tidak ada di `role_mapping`; lihat klaim di halaman *SSO* |

## 11. Uninstall

```bash
sudo systemctl disable --now degoploy
sudo rm /etc/systemd/system/degoploy.service /usr/local/bin/degoploy
sudo systemctl daemon-reload
# data & konfigurasi (berisi deploy key): hapus hanya jika sudah dibackup
sudo rm -rf /var/lib/degoploy /etc/degoploy
sudo userdel degoploy
```
