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

1. memasang `git`, `ssh` dan `sudo` bila belum ada (apt-get, dnf, yum, zypper, pacman, atau apk),
2. membuat user sistem `degoploy` untuk service, dan `degoploy-deploy` untuk git dan script deploy
   (keduanya tanpa login shell; lihat [bagian 6](#6-hak-akses-script-deploy)),
3. memasukkan user yang menjalankan `sudo` ke grup `degoploy` (`--operator NAMA` untuk user lain,
   `--no-operator` untuk melewati),
4. memasang binary ke `/usr/local/bin/degoploy`,
5. menulis `/etc/degoploy/degoploy.yaml` dan `/etc/degoploy/degoploy.env` **jika belum ada**, lalu
   memeriksanya (pada instalasi lama ia menambahkan `projects_dir`, `runtimes_dir` dan `deploy_user`
   bila belum ada),
6. menulis aturan sudo `/etc/sudoers.d/degoploy` (dicek dengan `visudo`) yang hanya mengizinkan
   `degoploy` menjalankan `degoploy run-step` sebagai `degoploy-deploy`,
7. membuat folder project `/srv/degoploy` milik `degoploy-deploy`, setgid, dengan grup web server
   (`www-data`, `nginx`, `apache` atau `http`, mana yang ada), dan memindahkan runtime ke
   `/var/lib/degoploy-runtimes`,
8. memasang `degoploy.service`, lalu `enable` + `start`, dan menunggu sampai service menjawab,
9. pada instalasi lama: memindahkan file milik `degoploy` di setiap folder kerja project ke
   `degoploy-deploy`, lalu memeriksa apakah ia bisa deploy di sana,
10. membuat admin pertama bila belum ada.

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
| `/srv/degoploy/<slug>/` | folder kerja project yang tidak memilih folder sendiri (`projects_dir`) | `2775` degoploy-deploy:www-data |
| `/var/lib/degoploy-runtimes/node/`, `php/` | versi Node.js dan PHP untuk deploy script (`runtimes_dir`, diunduh otomatis) | `0755` degoploy |
| `/var/lib/degoploy-deploy/` | HOME user `degoploy-deploy` (cache npm, Composer) | `0750` degoploy-deploy |
| `/etc/sudoers.d/degoploy` | izin `degoploy` menjalankan deployment sebagai `degoploy-deploy`; ditulis ulang setup | `0440` root |
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

1. **Folder kerja.** Kosongkan *Working Directory* dan degoploy memakai `/srv/degoploy/<slug>`
   (`projects_dir` di config). Folder itu dibuat sendiri saat deploy pertama, ikut grup web server, dan
   file hasil deploy bisa ditulis grup itu (umask service `0002`), jadi `storage/` atau `cache/` aplikasi
   tidak perlu di-`chown` lagi. Arahkan web server ke `/srv/degoploy/<slug>` (misalnya `…/public`).

   Dengan *Releases* menyala, tiap deployment dibangun di `releases/<id>` dan `current` baru dipindah
   ke sana setelah script sukses; arahkan web server ke `/srv/degoploy/<slug>/current` (misalnya
   `…/current/public`). Lihat [Releases and rollback](guide/releases.md).

   Folder lain, misalnya `/var/www/my-app` atau di home user, tetap boleh. git dan script berjalan
   sebagai `degoploy-deploy`, jadi user itu harus bisa menulis di sana, dan `degoploy` harus bisa
   membacanya (ia membaca `.nvmrc`, `composer.json` dan script dari repo). Saat project disimpan, degoploy
   memeriksa keduanya dan menampilkan perintah perbaikan bila belum bisa, misalnya:

   ```bash
   sudo install -d -o degoploy-deploy -g www-data -m 2775 /var/www/my-app
   # atau untuk folder yang sudah ada:
   sudo setfacl -R -m u:degoploy-deploy:rwX -m d:u:degoploy-deploy:rwX /var/www/my-app
   sudo setfacl -R -m u:degoploy:rX -m d:u:degoploy:rX /var/www/my-app
   ```

   Folder harus kosong atau sudah berupa repo git; folder berisi file lain ditolak agar tidak tertimpa.
   Folder di `/home` sering tertutup (`0750`), jadi `/srv/degoploy` biasanya pilihan yang lebih mudah.

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

git dan script deploy berjalan sebagai user `degoploy-deploy` (`deploy_user` di config), bukan sebagai
`degoploy`. Akibatnya script, termasuk script dari repo dan dependency seperti `postinstall` npm, tidak
bisa membaca `/var/lib/degoploy` (deploy key semua project, `secret.key`, database) maupun
`/etc/degoploy`, dan tidak bisa memakai admin socket. Caranya:

- service menjalankan `sudo -u degoploy-deploy degoploy run-step` dengan aturan di
  `/etc/sudoers.d/degoploy`; variabel dan secret dikirim lewat stdin, bukan argumen;
- deploy key project dipinjamkan lewat ssh-agent selama deployment (`GIT_SSH_COMMAND` menunjuk ke
  agent itu). Agent hanya menandatangani dan tidak bisa menyerahkan key-nya;
- script managed diberikan ke bash dari memori, jadi tidak perlu file yang bisa dibaca `degoploy-deploy`.

Untuk perintah yang butuh root, beri izin `sudo` spesifik untuk `degoploy-deploy` (jangan `ALL`):

```bash
sudo visudo -f /etc/sudoers.d/my-app
```

```sudoers
degoploy-deploy ALL=(root) NOPASSWD: /usr/bin/systemctl restart my-app, /usr/bin/systemctl reload nginx
```

Jangan menulis di `/etc/sudoers.d/degoploy`: file itu ditulis ulang setiap `degoploy setup`.

**Instalasi lama.** Dulu script berjalan sebagai `degoploy`. Saat `degoploy update` (atau `degoploy
setup`) pertama kali memindahkan script ke `degoploy-deploy`, aturan sudo untuk `degoploy` yang ia temukan
disalin sekali ke `/etc/sudoers.d/degoploy-deploy`, dan file milik `degoploy` di folder kerja project
dipindahkan ke `degoploy-deploy`. Setelah itu aturan lama untuk `degoploy` tidak dipakai script lagi dan
boleh dihapus. Untuk sementara kembali ke perilaku lama, kosongkan `deploy_user: ""` di config (tidak
disarankan).

Catatan:
- Jalankan aplikasi sebagai service systemd dan restart lewat `systemctl` dari script. Proses yang
  dijalankan di background langsung dari script (`nohup …&`) ikut berhenti saat degoploy di-restart.
- Memasukkan `degoploy-deploy` ke grup `docker` setara dengan akses root.
- Unit sengaja tidak memakai `NoNewPrivileges` (deployment dimulai lewat `sudo`) maupun `ProtectSystem`
  (agar penulisan ke `/var/www` tetap bisa).

### Node.js dan PHP untuk deploy script

Script berjalan sebagai user `degoploy-deploy`, jadi nvm atau phpenv milik user lain tidak terbaca. degoploy
mengelola runtime-nya sendiri, dan setiap project hanya memakai yang dibutuhkannya:

- **Node.js**: versi dari `.nvmrc`, `.node-version` atau `package.json`, dipasang dari halaman Runtimes
  dengan unduhan dari nodejs.org (dicek SHA256). pnpm dan yarn tersedia lewat corepack.
- **PHP**: versi dari `.php-version` atau `composer.json`. PHP yang sudah terpasang di server (misalnya
  `php8.3` dari apt atau PPA ondrej) dipakai lebih dulu, karena sama dengan PHP-FPM dan ekstensinya.
  Versi lain dipasang dari halaman Runtimes sebagai build statis dari static-php.dev (tanpa root,
  tanpa compile, dicek SHA256 untuk build terbaru). Composer
  diunduh dari getcomposer.org (dicek SHA256).

Di Project settings, setiap runtime bisa **Auto** (aktif bila repo memakainya), versi tertentu, atau
**Off**. Versi hanya dipasang di halaman **Runtimes** atau lewat CLI; project memilih dari versi yang
sudah terpasang, dan deployment tidak pernah mengunduh. Deployment yang butuh versi yang belum ada
akan gagal dan menyebut versi yang perlu dipasang:

```bash
degoploy php install 8.3     # node install 22, lts, 20.11.1
degoploy php default 8.3     # untuk project yang repo-nya tidak menyebut versi
degoploy node update         # naikkan setiap versi unduhan ke patch terbaru
```

Server tanpa akses internet bisa memakai mirror dengan struktur yang sama: `node_mirror`, `php_mirror`
dan `composer_url` di `degoploy.yaml` (atau `DEGOPLOY_NODE_MIRROR`, `DEGOPLOY_PHP_MIRROR`,
`DEGOPLOY_COMPOSER_URL`). Panduan lengkap: [docs/guide/runtimes.md](guide/runtimes.md).

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
| Deploy gagal `mkdir … permission denied` | User `degoploy-deploy` tidak bisa menulis ke `work_dir`; simpan ulang project untuk melihat perintah perbaikannya |
| Deploy gagal `sudo: a password is required` | Aturan `/etc/sudoers.d/degoploy` hilang atau `deploy_user` diganti; jalankan `sudo degoploy setup` |
| Script gagal `sudo: … password is required` untuk `systemctl` | Aturan sudo masih untuk `degoploy`; tulis untuk `degoploy-deploy` (bagian 6) |
| Deploy gagal `… is degoploy a member?` | `degoploy` belum masuk grup `degoploy-deploy`; jalankan `sudo degoploy setup` |
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
