# degoploy — Rencana Implementasi

Aplikasi untuk mengeksekusi script deploy saat ada webhook dari **GitHub** atau **Gitea**.
Setiap project punya folder kerja sendiri, SSH key sendiri (untuk private repo), trigger rules,
dan script deploy. Dilengkapi Web UI realtime, multi role, dan SSO (OIDC / Casdoor).
Hasil akhir: **1 binary statis**.

---

## 1. Alur Utama

```
GitHub/Gitea ──POST──▶ /hook/{project_id}
                          │ 1. verifikasi signature HMAC (secret per project)
                          │ 2. parse event (push / tag)
                          │ 3. cocokkan trigger rules (branch/tag, path, skip keyword)
                          ▼
                     Queue per project ──▶ Runner
                                              │ auto_pull: git clone / fetch + reset (SSH key project)
                                              │ jalankan script deploy (managed / repo) di work_dir
                                              ▼
                                   log ke file + status ke SQLite
                                              ▼
                                   WebSocket hub ──▶ Web UI (live)
```

## 2. Keputusan yang Sudah Disepakati

| Topik | Keputusan |
|---|---|
| Nama | `degoploy` |
| Bahasa backend | Go, `net/http` (routing Go 1.22+) |
| Database | SQLite via `modernc.org/sqlite` (pure Go, tanpa CGO) |
| Git | Panggil binary `git` dengan `GIT_SSH_COMMAND` per project |
| SSH key | ed25519 per project, digenerate otomatis (`golang.org/x/crypto/ssh`) |
| Lokasi script | **Keduanya**: `managed` (diedit dari UI) dan `repo` (file di dalam repo) |
| auto_pull | Default **aktif**; wajib aktif untuk mode script `repo` |
| Trigger | Rule per project: event, pola branch/tag, filter path, skip keyword |
| Role | Global: `admin`, `deployer`, `viewer`. Tabel per-project **disiapkan**, belum dipakai |
| Auth | Login lokal (break-glass) + **OIDC** (Casdoor) dengan mapping group → role |
| Frontend | **Svelte 5 + SvelteKit** (adapter-static, SPA), TypeScript, Tailwind v4, shadcn-svelte |
| Realtime | WebSocket (`github.com/coder/websocket`), pub/sub per topik |
| Distribusi | Frontend di-`go:embed` → 1 binary (`CGO_ENABLED=0`) |

## 3. Layout Data di Disk

Folder kerja project (`work_dir`, mis. `/var/www/app-a`) ditentukan per project.
Data internal aplikasi:

```
/var/lib/degoploy/                 (--data-dir)
├── degoploy.db
├── known_hosts                    # dipakai bersama, StrictHostKeyChecking=accept-new
└── projects/<slug>/
    ├── ssh/
    │   ├── id_ed25519             # 0600, tidak pernah ditampilkan di UI
    │   └── id_ed25519.pub         # dipasang sebagai Deploy Key
    ├── deploy.sh                  # script mode managed
    └── logs/<deployment_id>.log
```

## 4. Skema Database

```sql
projects (
  id, public_id UNIQUE,            -- id random untuk URL webhook
  slug UNIQUE, name,
  provider TEXT,                   -- 'github' | 'gitea'
  repo_url TEXT,
  work_dir TEXT,
  script_source TEXT,              -- 'managed' | 'repo'
  script_path TEXT,                -- untuk mode repo: relatif terhadap work_dir
  webhook_secret TEXT,
  auto_pull BOOLEAN DEFAULT 1,
  timeout_sec INTEGER DEFAULT 600,
  enabled BOOLEAN DEFAULT 1,
  created_at, updated_at
)

trigger_rules (
  id, project_id, name, enabled, position,
  event TEXT,                      -- 'push' | 'tag'
  ref_pattern TEXT,                -- glob: 'main', 'release/*', 'v*'
  paths_include TEXT,              -- JSON array glob, opsional
  paths_exclude TEXT,              -- JSON array glob, opsional
  skip_keyword TEXT,               -- default '[skip deploy]'
  extra_env TEXT                   -- JSON object
)

script_revisions (id, project_id, content, created_by, created_at)

deployments (
  id, project_id, rule_id NULL,
  trigger TEXT,                    -- 'webhook' | 'manual' | 'redeploy'
  triggered_by NULL,               -- user id untuk manual
  ref, commit_sha, commit_message, author,
  status TEXT,                     -- queued|running|success|failed|canceled|timeout|skipped
  exit_code INTEGER NULL,
  log_path TEXT,
  queued_at, started_at, finished_at
)

webhook_events (
  id, project_id, provider, event, delivery_id,
  ref, commit_sha, matched BOOLEAN, reason TEXT,
  deployment_id NULL, received_at
)

users (
  id, username UNIQUE, email, display_name,
  auth_provider TEXT,              -- 'local' | 'oidc'
  oidc_issuer, oidc_subject,       -- UNIQUE(oidc_issuer, oidc_subject)
  password_hash NULL,
  role TEXT,                       -- 'admin' | 'deployer' | 'viewer'
  disabled BOOLEAN, last_login_at, created_at
)

project_members (project_id, user_id, role)   -- akses per project untuk user dengan selected_projects

sessions (id_hash, user_id, csrf_token, expires_at, created_at, user_agent, ip)

audit_logs (id, user_id NULL, action, target_type, target_id, detail JSON, ip, created_at)
```

## 5. Webhook

- Endpoint: `POST /hook/{public_id}`. Body maksimal 5 MB.
- Verifikasi HMAC-SHA256 secara constant-time (`hmac.Equal`):
  - GitHub: `X-Hub-Signature-256: sha256=<hex>`
  - Gitea: `X-Gitea-Signature: <hex>` (juga terima `X-Hub-Signature-256`)
- Event header: `X-GitHub-Event` / `X-Gitea-Event`. `ping` → 200, `push` diproses, lainnya → 202 ignored.
- Push ke `refs/tags/*` diperlakukan sebagai event `tag`.
- Selalu balas cepat (202) setelah masuk antrian; deploy berjalan async.
- Semua webhook (cocok atau tidak) dicatat ke `webhook_events` beserta alasannya.

## 6. Trigger Rules

- Satu project bisa punya banyak rule. Deploy jalan jika **minimal satu rule cocok**
  (dievaluasi sesuai `position`, rule pertama yang cocok dipakai).
- Dalam satu rule, **semua kondisi harus terpenuhi**:
  1. `event` cocok (`push` untuk branch, `tag` untuk tag)
  2. `ref_pattern` (glob) cocok dengan nama branch/tag
  3. Jika `paths_include` diisi: minimal satu file berubah cocok
  4. Jika `paths_exclude` diisi: tidak semua file berubah ter-exclude
  5. Commit message tidak mengandung `skip_keyword`
- File berubah diambil dari `commits[].added/modified/removed`. Jika payload terpotong / kosong
  (force push, push besar), filter path dianggap **cocok** agar deploy tidak terlewat.
- UI menyediakan "Test rule" (tempel payload contoh → lihat rule mana yang cocok).

## 7. Script Deploy

| Mode | Lokasi | Catatan |
|---|---|---|
| `managed` | `<data>/projects/<slug>/deploy.sh` | Diedit via UI, setiap simpan → `script_revisions` |
| `repo` | `<work_dir>/<script_path>` | Dibaca setelah auto_pull. Path divalidasi tetap di dalam `work_dir` (tolak `..` dan symlink keluar) |

Env yang di-inject:

```
DEGOPLOY=1
DEPLOY_ID, DEPLOY_PROJECT, DEPLOY_RULE, DEPLOY_TRIGGER
DEPLOY_REF, DEPLOY_BRANCH | DEPLOY_TAG, DEPLOY_COMMIT, DEPLOY_AUTHOR
DEPLOY_WORKDIR
GIT_SSH_COMMAND="ssh -i <key> -o IdentitiesOnly=yes \
  -o UserKnownHostsFile=<data>/known_hosts -o StrictHostKeyChecking=accept-new"
+ extra_env dari rule
```

## 8. Runner

- Antrian per project: hanya 1 deploy berjalan per project.
- **Coalesce**: jika ada pending baru saat masih berjalan, pending lama → `skipped`, hanya yang terbaru dijalankan.
- Batas global deploy paralel (default 2, dapat dikonfigurasi).
- Langkah: auto_pull (`git clone` jika kosong, else `git fetch origin && git reset --hard <sha>`) → `bash <script>`.
- `Setpgid: true` → timeout / cancel membunuh seluruh process group.
- stdout+stderr → file log + broadcast ke WebSocket hub.
- Saat startup, deployment `running`/`queued` yang tersisa → `failed` (interrupted).

## 9. Auth & RBAC

### Role (global)

| Permission | admin | deployer | viewer |
|---|:-:|:-:|:-:|
| Lihat project, riwayat & log | ✅ | ✅ | ✅ |
| Deploy manual / cancel / redeploy | ✅ | ✅ | ❌ |
| Edit project, script, trigger rules | ✅ | ❌ | ❌ |
| Lihat public key / webhook secret | ✅ | ❌ | ❌ |
| Kelola user & role, audit log | ✅ | ❌ | ❌ |

- Semua pengecekan lewat satu fungsi `rbac.Can(user, action, projectID)`.
  User dengan `selected_projects` (deployer/viewer) hanya melihat project di `project_members`, dengan role
  deployer atau viewer per project; admin dan user lain memakai role global di semua project.

### Session

- Cookie `degoploy_session` (`HttpOnly`, `SameSite=Lax`, `Secure` jika HTTPS).
  Token disimpan di DB sebagai hash SHA-256.
- CSRF: token per session, dikirim client via header `X-CSRF-Token` untuk semua request non-GET.

### Login lokal

- Password bcrypt. User admin pertama dibuat via CLI: `degoploy user create --username admin --role admin`.
- Bisa dimatikan (`local_login: false`) jika SSO sudah stabil.

### OIDC (Casdoor)

- `github.com/coreos/go-oidc/v3` + `golang.org/x/oauth2`, Authorization Code + PKCE, validasi `state` & `nonce`.
- JIT provisioning: user dibuat saat login pertama, identitas = `issuer + sub`.
- Role dari claim group (`role_mapping`), disinkronkan setiap login (`sync_role_on_login`).
- Halaman debug (admin) menampilkan claim login terakhir untuk mencocokkan mapping.

Setup di Casdoor:
1. Buat Application `degoploy` di organisasi tim.
2. Redirect URL: `https://<host>/auth/oidc/callback`.
3. Grant type `authorization_code`; Token format `JWT` (bukan `JWT-Standard`) agar `groups` ikut.
4. Buat group (mis. `degoploy-admins`, `devops`) dan masukkan user.

## 10. Konfigurasi

File `degoploy.yaml` (path via `--config`), semua nilai bisa di-override env `DEGOPLOY_*`,
dan nilai `${VAR}` di-expand dari environment.

```yaml
listen: ":8080"
data_dir: /var/lib/degoploy
base_url: https://deploy.example.com
max_parallel: 2
trusted_proxies: [127.0.0.1/32]
local_login: true

oidc:
  enabled: false
  provider_name: Casdoor
  issuer: https://sso.example.com
  client_id: xxxxx
  client_secret: ${DEGOPLOY_OIDC_SECRET}
  redirect_url: https://deploy.example.com/auth/oidc/callback
  scopes: [openid, profile, email]
  groups_claim: groups
  role_mapping:
    "org/degoploy-admins": admin
    "org/devops": deployer
    "*": viewer
  sync_role_on_login: true
```

## 11. API & Realtime

### REST (`/api/v1`)

```
GET    /me                         POST /auth/login   POST /auth/logout
GET    /auth/providers             (local / oidc tersedia?)
GET    /projects                   POST /projects
GET    /projects/{id}              PATCH /projects/{id}   DELETE /projects/{id}
GET    /projects/{id}/secrets      (webhook url+secret, public key — admin)
POST   /projects/{id}/regenerate-key    POST /projects/{id}/regenerate-secret
POST   /projects/{id}/test-connection   (git ls-remote)
GET    /projects/{id}/script       PUT /projects/{id}/script
GET    /projects/{id}/script/revisions
GET    /projects/{id}/rules        POST/PATCH/DELETE ...   POST /projects/{id}/rules/test
GET    /projects/{id}/deployments  POST /projects/{id}/deploy
GET    /deployments/{id}           POST /deployments/{id}/cancel  POST /deployments/{id}/redeploy
GET    /deployments/{id}/log
GET    /projects/{id}/webhook-events
GET    /users   POST /users   PATCH /users/{id}
GET    /audit
```

OIDC: `GET /auth/oidc/login`, `GET /auth/oidc/callback` (di luar `/api`, berupa redirect).

### WebSocket (`/ws`)

Satu koneksi per tab, pub/sub per topik.

```jsonc
// client → server
{"op":"subscribe","topic":"deployment:42:log","offset":0}
{"op":"subscribe","topic":"projects"}
{"op":"unsubscribe","topic":"deployment:42:log"}

// server → client
{"topic":"deployment:42:log","type":"log.chunk","offset":10240,"data":"..."}
{"topic":"deployment:42","type":"deployment.status","data":{...}}
{"topic":"projects","type":"project.updated","data":{...}}
{"topic":"projects","type":"webhook.received","data":{...}}
```

- Auth via session cookie + cek `Origin` saat upgrade; RBAC dicek per subscribe.
- Log: saat subscribe, server kirim isi file dari `offset` lalu lanjut streaming (tanpa celah/dobel).
- Buffered channel per client; client lambat diputus. Ping/pong heartbeat.
- Client: reconnect exponential backoff, resubscribe otomatis dengan offset terakhir.
- Event status meng-invalidate cache TanStack Query di frontend.

## 12. Frontend

| Bagian | Pilihan |
|---|---|
| Framework | Svelte 5 (runes) + SvelteKit `adapter-static`, `ssr = false`, fallback `index.html` |
| Bahasa | TypeScript |
| Styling | Tailwind CSS v4 + shadcn-svelte |
| Editor script | CodeMirror 6 (bash) |
| Log viewer | xterm.js (ANSI color, performa log besar) |
| Data fetching | TanStack Query |
| Tipe API | Digenerate dari struct Go via `tygo` |

```
web/src/
├── routes/
│   ├── +layout.svelte          # shell, sidebar, auth guard
│   ├── login/
│   ├── projects/ (+page, new/, [id]/{settings,rules,script,deployments/[did]})
│   ├── users/
│   └── audit/
└── lib/
    ├── api/        # fetch client + tipe hasil tygo
    ├── ws/         # koneksi WS, subscribe, reconnect
    ├── stores/     # session, permission helper
    └── components/ # ui/ (shadcn), LogViewer, ScriptEditor, RuleEditor, StatusBadge, CopyField
```

## 13. Struktur Repo

```
degoploy/
├── cmd/degoploy/main.go       # serve, user create, version
├── internal/
│   ├── config/   store/   sshkey/   gitops/
│   ├── webhook/               # github, gitea, verify, payload
│   ├── trigger/               # rule matcher
│   ├── runner/                # queue, executor
│   ├── realtime/              # WS hub
│   ├── auth/                  # local, oidc, session, csrf, rbac
│   ├── audit/
│   ├── api/                   # handler JSON /api/v1
│   └── server/                # wiring router + middleware
├── web/                       # SvelteKit app; web/embed.go meng-embed web/build
├── deploy/degoploy.service
├── docs/PLAN.md
└── Makefile                   # dev, types, web, build, test
```

## 14. Keamanan

- Jalankan sebagai user khusus `degoploy` (bukan root). Butuh restart service → aturan sudoers spesifik.
- Private key `0600`, direktori data `0700`. Private key tidak pernah keluar lewat API.
- Webhook secret 32 byte random; `public_id` project random.
- Rate limit di `/hook/*` dan `/api/v1/auth/login`.
- Disarankan di belakang reverse proxy (Caddy/Nginx) + HTTPS.
- Header keamanan: CSP, `X-Content-Type-Options`, `Referrer-Policy`, `X-Frame-Options`.

## 15. Tahapan

| # | Tahap | Status |
|---|---|---|
| 1 | Fondasi backend: config, SQLite + migrasi, login lokal, session, CSRF, RBAC | ✅ |
| 2 | Fondasi frontend: SvelteKit + Tailwind + shadcn-svelte, layout, login, tygo, embed, `make build` | ✅ |
| 3 | Project: API + UI CRUD, SSH key, secret, script managed/repo + revisi | ✅ |
| 4 | Webhook + trigger rules (+ unit test), log webhook events | ✅ |
| 5 | Runner + realtime: antrian, eksekusi, WS hub, LogViewer live | ✅ |
| 6 | User management + audit log | ✅ |
| 7 | OIDC Casdoor: login, JIT provisioning, role mapping, debug claim | ✅ |
| 8 | Packaging: systemd unit, README setup | ✅ |

### Catatan implementasi (tahap 1–2)

- Versi aktual: Go 1.27, SvelteKit 3 + Svelte 5, Vite 8, Tailwind v4, shadcn-svelte (style vega, zinc, lucide).
- SvelteKit 3: config ada di `web/vite.config.ts` (tidak ada `svelte.config.js`); import internal memakai
  subpath `#lib/...` (bukan `$lib`); link wajib lewat `resolve()` dari `$app/paths` (aturan eslint).
- Output build UI langsung ke `internal/web/dist` (di-embed). `go.mod` memakai `ignore ./web/node_modules`.
- CSP SPA berbasis hash lewat `<meta>` (diset di `vite.config.ts`); respons API memakai CSP `default-src 'none'`.
- Login lokal: rate limit 10 percobaan lalu 10/menit per IP; body wajib `application/json` (mencegah CSRF form login).
- Session: token acak 32 byte, disimpan sebagai SHA-256, sliding expiry ketika sisa TTL < 50%.

### Catatan implementasi (tahap 3)

- `internal/project` mengelola baris DB + folder `<data>/projects/<slug>` (0700): deploy key, `deploy.sh`, `logs/`.
  Slug tidak bisa diubah. Hapus project tidak menyentuh `work_dir`.
- Validasi: `repo_url` hanya `git@host:…`, `ssh://`, `https://`/`http://`, tidak boleh diawali `-`;
  `work_dir` absolut, bukan `/`, tidak tumpang-tindih dengan data dir maupun `work_dir` project lain.
- git dijalankan dengan `ssh -F /dev/null`, `BatchMode`, `IdentitiesOnly`, known_hosts milik degoploy,
  `GIT_ALLOW_PROTOCOL=ssh:https:http`, dan `--` sebelum URL.
- Script repo di-resolve dengan `EvalSymlinks` dan ditolak bila keluar dari `work_dir`.
- Izin: lihat script = `project.deploy` (deployer & admin); ubah script/project = `project.edit`;
  deploy key & webhook secret = `project.secrets` (admin). Semua perubahan dan akses secret masuk audit log.

### Catatan implementasi (tahap 4)

- `POST /hook/{public_id}`: tanpa session; autentikasi = HMAC-SHA256 dengan secret project.
  GitHub wajib `X-Hub-Signature-256`; Gitea menerima `X-Gitea-Signature` atau `X-Hub-Signature-256`.
  Payload JSON maupun form-encoded (`payload=`, default lama GitHub) didukung. Rate limit 120/menit per IP.
- Balasan: `pong` (ping), `queued`, `skipped`, `ignored`, `rejected` + alasan; tampil juga di log delivery GitHub/Gitea.
- Semua delivery (kecuali `public_id` tak dikenal) dicatat di `webhook_events`, maksimal 200 per project.
- Pola ref dipisah koma; glob `*`, `**`, `?`. Pola path tanpa `/` juga mencocokkan nama file di folder mana pun.
- Saat membuat project, isian "Branch to deploy" membuat rule pertama `Push to <branch>`.
- Nilai `extra_env` disamarkan untuk user tanpa izin `project.edit`. Nama variabel `DEPLOY_*`, `DEGOPLOY*`, dan
  `GIT_SSH_COMMAND` tidak boleh di-override.
- Webhook yang cocok membuat baris `deployments` berstatus `queued`; eksekusinya menunggu runner (tahap 5).
- Guard halaman frontend menunggu `parent()` agar `/me` selesai dimuat sebelum cek izin.

### Catatan implementasi (tahap 5)

- `internal/deploy.Service`: satu loop per project, `max_parallel` slot global. Push baru saat project sibuk
  menggantikan antrian lama (yang lama → `skipped`). Cancel: SIGTERM ke process group, SIGKILL setelah 10 detik.
- Langkah auto_pull: `git init` + `remote add` (folder kosong/belum ada) atau `remote set-url`, lalu
  `git fetch --force --prune --no-tags origin -- <ref>`, `git checkout --force --detach <sha|FETCH_HEAD>`,
  dan `git submodule update --init --recursive` bila ada `.gitmodules`. Folder berisi file tanpa `.git` ditolak.
  `git clean` sengaja tidak dijalankan (file seperti `.env`, `vendor/`, upload tetap aman).
- Script: `bash --noprofile --norc <script>` di `work_dir`, env tanpa `DEGOPLOY_*` milik server,
  `extra_env` rule lebih dulu lalu `DEPLOY_*` (tidak bisa ditimpa). Proses latar belakang yang sengaja
  ditinggal script (mis. `nohup`) tidak dibunuh saat deploy sukses.
- Log: `<data>/projects/<slug>/logs/<id>.log`, maks 10 MiB per deployment, 100 deployment terakhir per project disimpan.
- Shutdown: deploy yang berjalan diberi 60 detik untuk selesai, lalu dibatalkan; antrian dilanjutkan saat start.
  Status `running` yang tertinggal karena crash → `failed` saat start.
- WebSocket `/ws`: autentikasi cookie session + cek Origin (host sama atau `base_url`), sesi dicek ulang tiap menit.
  Topik `events` (deployment.updated, webhook.received, difilter izin per project) dan `deployment:<id>:log`
  (backfill dari file + live, chunk base64 dengan offset byte; client membuang duplikat berdasarkan offset).
- Frontend: satu koneksi per tab, reconnect exponential backoff, refetch semua query setelah reconnect.
  Cache deployment tidak pernah mundur status (queued → running → selesai) walau respons HTTP dan event WS balapan.

### Catatan implementasi (tahap 6)

- API `/api/v1/users` (izin `user.manage`): buat user lokal, ubah nama/email/role/nonaktif, reset password,
  hapus. Admin tidak bisa mengubah role sendiri, menonaktifkan, atau menghapus akunnya sendiri; perubahan yang
  menyisakan nol admin aktif ditolak.
- Menonaktifkan user / reset password menghapus semua session user itu (langsung ter-logout). Perubahan role
  berlaku seketika untuk request HTTP dan dalam ≤1 menit untuk koneksi WebSocket yang sudah terbuka.
- `POST /api/v1/me/password`: ganti password sendiri (butuh password lama, kena rate limit login); session lain
  di-logout, session saat ini tetap.
- `GET /api/v1/audit` (izin `audit.view`): filter `action` (prefix, mis. `user.`), `user`, paginasi kursor `before`.
  Detail perubahan disimpan sebagai `{"changes": {field: [lama, baru]}}` dan ditampilkan `field: lama → baru`.
- Tipe TypeScript: mode default tygo (field pointer `?:`), field response yang bisa null ditandai eksplisit
  `tstype:"T | null,required"`.

### Catatan implementasi (tahap 7)

- `GET /auth/oidc/login` → Authorization Code + PKCE (S256), `state` + `nonce` acak. Data login yang sedang
  berjalan disimpan di memori (10 menit) dan dikaitkan ke browser lewat cookie `degoploy_oidc`
  (`HttpOnly`, `SameSite=Lax`, path `/auth/oidc`); restart degoploy membatalkan login yang sedang di tengah jalan.
- `GET /auth/oidc/callback`: cek cookie + `state` (constant-time), tukar code, verifikasi ID token
  (signature JWKS, issuer, audience, expiry, nonce), gabungkan klaim userinfo yang belum ada di ID token.
- Group dari klaim `groups_claim`: daftar string, satu string, atau objek `{owner, name}` → `owner/name`.
  Role = role tertinggi dari group yang ada di `role_mapping`; `"*"` jadi fallback; tanpa match → ditolak.
- Akun dicari lewat `issuer + sub` saja; user baru dibuat otomatis (username dari `preferred_username` /
  email, diberi akhiran bila bentrok). Email/nama selalu diperbarui; role diperbarui jika `sync_role_on_login`.
- Kegagalan diarahkan ke `/login?error=…` dengan pesan yang bisa dibaca dan dicatat di audit
  (`auth.login_failed`, provider `oidc`). Discovery dilakukan lazy, jadi degoploy tetap jalan saat Casdoor down.
- Halaman **SSO** (admin, `GET /api/v1/auth/sso`): konfigurasi tanpa secret, tabel group → role, dan 20 percobaan
  login SSO terakhir beserta klaim mentahnya (di memori, hilang saat restart).
- Test memakai `internal/auth/oidctest`: IdP palsu (discovery, authorize, token+PKCE, userinfo, JWKS, RS256).

### Catatan implementasi (tahap 8)

- `make dist` → `release/degoploy-<versi>-linux-{amd64,arm64}.tar.gz` + `SHA256SUMS`; isi: binary, `install.sh`,
  `degoploy.service`, `degoploy.example.yaml`, `README.md`, `INSTALL.md`.
- `deploy/install.sh` (root): user sistem `degoploy`, binary ke `/usr/local/bin` (ganti atomik), config &
  env di `/etc/degoploy` (tidak pernah ditimpa), `config check`, `--admin` opsional, pasang + enable + start/restart
  unit. Idempoten; dijalankan ulang = upgrade.
- `deploy/degoploy.service`: `StateDirectory=degoploy` (0700), `EnvironmentFile=/etc/degoploy/degoploy.env`,
  `KillMode=mixed`, `TimeoutStopSec=90s` (60 detik untuk deploy yang berjalan). Hardening ringan saja;
  `NoNewPrivileges`/`ProtectSystem` dibiarkan opsional karena script deploy sering butuh `sudo` dan menulis ke `/var/www`.
- CLI baru: `degoploy backup` (tar.gz: snapshot DB via `VACUUM INTO` + `projects/` + `known_hosts`, aman saat
  service jalan) dan `degoploy config check` (validasi + uji discovery OIDC).
- Diuji di container Ubuntu 24.04 bersih (tanpa systemd): install, admin, deploy sungguhan sebagai user `degoploy`,
  backup, dan install ulang. Jalur systemd di installer belum diuji di mesin dengan systemd.

## 16. Rencana Lanjutan (opsional)

- Notifikasi Telegram / Discord / Slack
- Multi environment per project, rollback ke commit sebelumnya
- Dukungan GitLab, trigger dari event release
