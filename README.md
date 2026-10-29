# degoploy

Webhook deploy runner untuk GitHub dan Gitea. Satu binary Go dengan Web UI (SvelteKit) ter-embed:
trigger rules per project, deploy key SSH per project, live log, multi role, dan SSO (OIDC/Casdoor).

> Status: tahap 1–8 selesai. Instalasi di server: [docs/INSTALL.md](docs/INSTALL.md).
> Rencana dan catatan implementasi: [docs/PLAN.md](docs/PLAN.md).

**User guide** — what you can do in degoploy and how (English, also in the app under **Docs**):
[docs/guide](docs/guide/README.md).

## Kebutuhan

- Go 1.27+ (lihat `go.mod`)
- Node.js 24+ dan npm (hanya untuk build UI)
- `git` di server (dipakai saat deploy)
- `make` (opsional, untuk shortcut di bawah)

## Build

```bash
make build          # tygo -> build UI -> bin/degoploy (CGO_ENABLED=0, statis)
make dist           # tarball rilis linux/amd64 + linux/arm64 di release/
```

Rilis: push tag versi, GitHub Actions (`.github/workflows/release.yml`) membangun, menguji, dan
mengunggah tarball + `SHA256SUMS` ke GitHub Releases, tempat `install.sh` dan `degoploy update` mengunduh.

```bash
git tag v1.0.0 && git push origin v1.0.0
```

Tanpa `make`:

```bash
go tool tygo generate
(cd web && npm ci && npm run build)      # output ke internal/web/dist
CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o bin/degoploy ./cmd/degoploy
```

## Menjalankan

```bash
cp degoploy.example.yaml degoploy.yaml   # sesuaikan
./bin/degoploy user create --config degoploy.yaml --username admin --role admin
./bin/degoploy serve --config degoploy.yaml
```

Di server produksi cukup satu perintah; ia mengunduh rilis, memasang service systemd, dan membuat
admin pertama (detail di [docs/INSTALL.md](docs/INSTALL.md)):

```bash
curl -fsSL https://raw.githubusercontent.com/Degovan/degoploy/main/install.sh | sh
```

Upgrade: `degoploy update`.

Perintah CLI lain: `setup`, `update`, `status`, `user` (menu interaktif), `user passwd|role|disable|enable|list`,
`backup`, `config check`, `version`; lihat `degoploy help`. Flag yang tidak diberikan ditanyakan lewat form
di terminal. Saat service jalan, CLI memakai socket admin-nya, jadi anggota grup `degoploy` tidak perlu
`sudo`. Semua opsi config bisa
di-override lewat env `DEGOPLOY_*` (mis. `DEGOPLOY_LISTEN`, `DEGOPLOY_DATA_DIR`,
`DEGOPLOY_OIDC_CLIENT_SECRET`). Variabel `DEGOPLOY_*` tidak pernah diteruskan ke script deploy.

## SSO dengan Casdoor

1. **Di Casdoor**, buat *Application* di organisasi tim:
   - *Redirect URLs*: `https://<host-degoploy>/auth/oidc/callback`
     (sama persis dengan `oidc.redirect_url`; halaman **SSO** di degoploy menampilkannya).
   - *Grant types*: `Authorization Code`.
   - *Token format*: `JWT`. Format minimal (mis. `JWT-Standard`) bisa tidak menyertakan group;
     degoploy juga membaca endpoint userinfo, tapi `JWT` paling aman.
   - Catat *Client ID* dan *Client secret*.
2. Buat group (mis. `degoploy-admins`, `devops`) dan masukkan user ke dalamnya.
3. **Di `degoploy.yaml`**:

   ```yaml
   base_url: https://deploy.example.com
   oidc:
     enabled: true
     provider_name: Casdoor
     issuer: https://sso.example.com          # origin Casdoor, tanpa path
     client_id: <client id>
     client_secret: ${DEGOPLOY_OIDC_SECRET}  # isi lewat environment
     redirect_url: https://deploy.example.com/auth/oidc/callback
     scopes: [openid, profile, email]
     groups_claim: groups
     role_mapping:                           # role tertinggi dari group user yang dipakai
       "my-org/degoploy-admins": admin
       "my-org/devops": deployer
       "*": viewer                           # hapus agar user di luar group ditolak
     sync_role_on_login: true                # role diperbarui dari group setiap login
   ```

4. Restart degoploy, lalu login lewat **Continue with Casdoor**. Buka menu **SSO** (admin) untuk melihat
   klaim dan group yang benar-benar dikirim Casdoor (mis. `my-org/devops`), lalu sesuaikan `role_mapping`.

Catatan:
- Akun SSO dikaitkan lewat `issuer + sub`, tidak pernah lewat username/email, jadi user Casdoor tidak bisa
  mengambil alih akun lokal yang username-nya sama (akun baru akan diberi akhiran, mis. `dewi-2`).
- Simpan minimal satu admin lokal sebagai cadangan jika Casdoor down; setelah itu `local_login: false`
  boleh dipakai untuk mewajibkan SSO.
- Logout di degoploy hanya mengakhiri sesi degoploy, bukan sesi Casdoor.

## Development

```bash
make dev    # API di 127.0.0.1:8090 (degoploy.dev.yaml) + Vite di 0.0.0.0:5173 dengan proxy /api, /auth, /ws, /hook
make test   # go test ./...
make check  # go vet + svelte-check + prettier + eslint
make types  # regenerate web/src/lib/api/types.gen.ts dari internal/api/types.go
```

Buka http://170.80.9.6:5173 (atau http://localhost:5173) saat dev. Buat user dulu dengan
`go run ./cmd/degoploy user create --config degoploy.dev.yaml --username admin`.
Untuk mencoba SSO saat dev, `redirect_url` harus memakai alamat dev server, mis.
`http://170.80.9.6:5173/auth/oidc/callback` (Vite meneruskan `/auth` ke API).

## Struktur

```
cmd/degoploy/        CLI: serve, setup, status, user, backup, config check, version
internal/admin/      socket admin untuk CLI (user, status, backup, config check) + fallback in-process
deploy/              degoploy.service + template config (di-embed untuk degoploy setup)
install.sh           installer satu baris (curl | sh): unduh rilis, cek SHA256, jalankan degoploy setup
internal/config/     YAML + env
internal/store/      SQLite (modernc, tanpa CGO) + migrasi embed
internal/auth/       password, session cookie, CSRF, RBAC, OIDC (oidctest: IdP palsu untuk test)
internal/project/    project, deploy key, script managed/repo
internal/webhook/    verifikasi HMAC + parsing payload GitHub/Gitea
internal/trigger/    glob + evaluasi trigger rules
internal/deploy/     antrian per project + runner (git, script, log)
internal/realtime/   hub WebSocket per topik
internal/api/        JSON API /api/v1, /hook, /ws, /auth/oidc (types.go -> TypeScript via tygo)
internal/server/     wiring router + middleware + shutdown
internal/web/        embed SPA (dist/ hasil build web/)
web/                 SvelteKit 3 + Svelte 5 + Tailwind v4 + shadcn-svelte
```
