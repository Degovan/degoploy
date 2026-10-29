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

## 2. Instal

Satu perintah di server, sebagai user biasa yang boleh `sudo`:

```bash
curl -fsSL https://raw.githubusercontent.com/irsyadulibad/degoploy/main/install.sh | sh
```

Script itu mengunduh rilis terbaru untuk arsitektur server (amd64/arm64) dari GitHub Releases,
mencocokkan SHA256-nya, lalu menjalankan `degoploy setup` lewat `sudo`. Setup menanyakan alamat listen,
URL publik, apakah Anda boleh menjalankan `degoploy` tanpa `sudo`, dan bila perlu memasang `git` +
`ssh`. Setelah service jalan, ia meminta admin pertama dan menampilkan alamat web. Tidak perlu clone,
compile, atau mengatur env.

Versi tertentu: `… | sh -s -- --version v1.2.3`. Untuk otomasi (tanpa pertanyaan), unduh script-nya
dulu lalu kirim password admin lewat stdin:

```bash
curl -fsSLo install.sh https://raw.githubusercontent.com/irsyadulibad/degoploy/main/install.sh
echo "$ADMIN_PW" | sh install.sh --yes --base-url https://deploy.example.com --admin admin
```

Tanpa internet di server: unduh `degoploy-linux-amd64.tar.gz` dari halaman
[Releases](https://github.com/irsyadulibad/degoploy/releases), salin ke server, ekstrak, lalu jalankan
`./degoploy` di terminal. Binary yang belum terpasang menawarkan instalasi yang sama.

`degoploy setup` melakukan semuanya:

1. memasang `git` dan `ssh` bila belum ada (apt-get, dnf, yum, zypper, pacman, atau apk),
2. membuat user sistem `degoploy` (tanpa login shell),
3. memasukkan user yang menjalankan `sudo` ke grup `degoploy` (`--operator NAMA` untuk user lain,
   `--no-operator` untuk melewati),
4. memasang binary ke `/usr/local/bin/degoploy`,
5. menulis `/etc/degoploy/degoploy.yaml` dan `/etc/degoploy/degoploy.env` **jika belum ada**, lalu
   memeriksanya,
6. memasang `degoploy.service`, lalu `enable` + `start`, dan menunggu sampai service menjawab,
7. membuat admin pertama bila belum ada.

Opsi lain: `--listen 127.0.0.1:9000`, `--no-start`. Setiap langkah memeriksa dulu, jadi menjalankan
`sudo degoploy setup` lagi juga memperbaiki izin file dan unit yang berubah.

Grup baru aktif setelah logout lalu login lagi (atau `newgrp degoploy`).

### Build sendiri

```bash
make dist            # release/degoploy-linux-{amd64,arm64}.tar.gz + SHA256SUMS
```

## 3. Lokasi file

| Path | Isi | Izin |
|---|---|---|
| `/usr/local/bin/degoploy` | binary | `0755` root |
| `/etc/degoploy/` | folder konfigurasi | `0750` degoploy:degoploy |
| `/etc/degoploy/degoploy.yaml` | konfigurasi (halaman *SSO* dan *Email* menyimpan ke sini) | `0640` degoploy:degoploy |
| `/etc/degoploy/degoploy.env` | secret (`DEGOPLOY_OIDC_SECRET=…`), dibaca systemd | `0600` root |
| `/var/lib/degoploy/` | database, deploy key, script, log | `0700` degoploy |
| `/var/lib/degoploy/node/` | versi Node.js untuk deploy script (diunduh otomatis) | `0755` degoploy |
| `/run/degoploy/admin.sock` | socket untuk CLI `degoploy` | `0660` degoploy:degoploy |
| `/etc/systemd/system/degoploy.service` | unit systemd | `0644` |

Ubah konfigurasi lalu cek dan restart:

```bash
degoploy config check
sudo systemctl restart degoploy
```

### Perintah `degoploy` tanpa sudo

CLI berbicara dengan service yang sedang berjalan lewat `/run/degoploy/admin.sock`. Root dan anggota
grup `degoploy` bisa memakainya tanpa `sudo` dan tanpa `--config`:

```bash
degoploy status                  # service jalan? alamat, jumlah user dan project
degoploy user                    # menu: buat user, set password, ganti role, nonaktifkan
degoploy user create             # flag yang tidak diberikan akan ditanyakan
degoploy backup --output ~/degoploy.tar.gz
degoploy config check
```

Tambahkan operator lain dengan `sudo usermod -aG degoploy NAMA` (berlaku setelah login ulang).
Anggota grup ini setara admin degoploy: mereka bisa membuat admin dan mengambil backup berisi deploy key.
Perubahan dari CLI tercatat di *Audit log* dengan nama user Linux-nya, misalnya `budi (CLI)`.

Bila service sedang mati, CLI membuka data secara langsung; itu butuh akses user `degoploy`, jadi
jalankan `sudo -u degoploy degoploy …` atau nyalakan service dulu. Di skrip, beri semua flag dan kirim
password lewat stdin: `echo "$PW" | degoploy user passwd --username budi`.

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

### Node.js untuk deploy script

Script berjalan sebagai user `degoploy`, jadi nvm milik user lain tidak terbaca. degoploy mengelola
Node.js sendiri: setiap project memakai versi dari `.nvmrc`, `.node-version` atau `engines.node`
(atau versi yang dipilih di Project settings), dan versi itu diunduh dari nodejs.org saat pertama kali
dibutuhkan, dicek SHA256-nya, lalu disimpan di `/var/lib/degoploy/node/`. `node`, `npm`, `npx`,
`pnpm` dan `yarn` langsung ada di `PATH` script tanpa memuat nvm. Kelola versinya di halaman
**Node.js** atau lewat CLI:

```bash
degoploy node install 22     # atau lts, 20.11.1
degoploy node default 22     # untuk project yang repo-nya tidak menyebut versi
degoploy node update         # naikkan setiap versi ke patch terbaru
```

Server tanpa akses ke nodejs.org bisa memakai mirror dengan struktur yang sama:
`node_mirror: https://mirror.example.com/nodejs` di `degoploy.yaml` (atau `DEGOPLOY_NODE_MIRROR`).
Panduan lengkap: [docs/guide/nodejs.md](guide/nodejs.md).

## 7. SSO (Casdoor)

Lihat bagian *SSO dengan Casdoor* di README. Secret masuk ke `/etc/degoploy/degoploy.env`:

```bash
echo 'DEGOPLOY_OIDC_SECRET=…' | sudo tee -a /etc/degoploy/degoploy.env
sudo systemctl restart degoploy
```

## 8. Upgrade

```bash
degoploy update            # unduh rilis terbaru, cek SHA256, ganti binary, restart service
degoploy update --check    # hanya cek apakah ada versi baru
```

Menjalankan ulang one-liner di bagian 2 juga meng-upgrade. Bila binary baru Anda taruh sendiri di
`/usr/local/bin` (`sudo install -m 0755 degoploy /usr/local/bin/`), jalankan `degoploy setup`; ia melihat
service masih memakai versi lama dan menawarkan restart.

Konfigurasi dan data tidak disentuh; migrasi database berjalan otomatis saat start. Deploy yang sedang
berjalan diberi waktu 60 detik untuk selesai, antrian dilanjutkan setelah restart.

## 9. Backup & restore

```bash
degoploy backup --output ~/degoploy-backup-$(date +%F).tar.gz      # tambahkan --no-logs untuk lebih kecil
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
degoploy status
degoploy config check
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
| CLI: `may not manage degoploy` | User Anda belum di grup `degoploy`: `sudo usermod -aG degoploy $USER`, lalu login ulang |
| CLI: `login session started before that` | Sudah di grup, tapi sesi lama: logout/login atau `newgrp degoploy` |
| Halaman *SSO*/*Email* gagal menyimpan | `/etc/degoploy` bukan milik `degoploy`; jalankan `sudo degoploy setup` lagi |

## 11. Uninstall

```bash
sudo systemctl disable --now degoploy
sudo rm /etc/systemd/system/degoploy.service /usr/local/bin/degoploy
sudo systemctl daemon-reload
# data & konfigurasi (berisi deploy key): hapus hanya jika sudah dibackup
sudo rm -rf /var/lib/degoploy /etc/degoploy
sudo userdel degoploy
```
