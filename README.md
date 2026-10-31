# degoploy

degoploy runs your deploy script whenever someone pushes to a GitHub or Gitea repository. It is a single
Go binary with the web UI built in: you register a project, choose which pushes may deploy, write the
script, and watch every deployment and its log live in the browser.

- **Webhooks** from GitHub and Gitea, verified with a per-project HMAC secret.
- **Trigger rules** per project: branches or tags (globs), changed paths, and skip keywords.
- **Deploy keys**: each project gets its own SSH key for private repositories.
- **Deploy scripts** kept in degoploy with revisions, or read from the repository, plus variables and
  encrypted secrets. They run as a user of their own, which cannot read deploy keys or secrets of
  other projects.
- **Runtimes**: Node.js and PHP versions per project, read from `.nvmrc`, `.php-version` or
  `composer.json` like nvm and phpenv do. The server's own versions are used where they fit; others are
  downloaded on first use. pnpm and yarn come through corepack, and Composer is included.
- **Live logs** over WebSocket, a queue per project, cancel and redeploy.
- **Releases**: each deploy builds in its own folder, `current` switches only when the script succeeds, and a rollback is one click. An optional health check switches back on its own when the new release does not answer.
- **Users and roles** (admin, deployer, viewer), optionally limited to selected projects, invitations by email, and an audit log.
- **Single sign-on** with any OIDC provider (tested with Casdoor), with groups mapped to roles.
- **A CLI** that installs, updates and manages degoploy on the server without `sudo`.

**User guide:** what you can do in degoploy and how. It is in [docs/guide](docs/guide/README.md) and
in the app under **Docs**.

## Install

On a Linux server (amd64 or arm64) with systemd, as a user who may use `sudo`:

```bash
curl -fsSL https://raw.githubusercontent.com/irsyadulibad/degoploy/main/install.sh | sh
```

The script downloads the latest release and checks its SHA256 sum, then runs `degoploy setup`. Setup asks
a few questions, installs git and ssh if they are missing, and creates a `degoploy` system user, the
config in `/etc/degoploy`, the projects folder `/srv/degoploy` and a systemd service. It starts the
service, asks for the first admin, and prints the web address. Put a reverse proxy with HTTPS in front
of it.

To update a server later:

```bash
degoploy update
```

[docs/INSTALL.md](docs/INSTALL.md) covers the details (in Indonesian): unattended installs, the reverse
proxy, file locations, permissions for deploy scripts, backups and troubleshooting.

## Managing a server

Root and members of the `degoploy` group run these without `sudo`. They talk to the running service
through `/run/degoploy/admin.sock`, and changes appear in the audit log under the Linux user's name.

```bash
degoploy                 # in a terminal: install on a new server, otherwise show the status
degoploy status          # is the service running, where, and how many users and projects
degoploy user            # menu: create a user, set a password, change a role, disable or enable
degoploy node|php       # runtime versions for deploy scripts: install, default, update
degoploy backup          # archive of the database, deploy keys, scripts and logs
degoploy config check    # validate the configuration
degoploy update          # download the latest release and restart on it
degoploy help            # every command and flag
```

Flags left out are asked for in a terminal. Scripts pass every flag and pipe passwords on stdin, for
example `echo "$PW" | degoploy user passwd --username budi`. Membership of the `degoploy` group is
as powerful as an admin account, so add only people you would make admins.

## Configuration

`/etc/degoploy/degoploy.yaml` is written by setup. See [degoploy.example.yaml](degoploy.example.yaml)
for every option. Each can be overridden with a `DEGOPLOY_*` environment variable, for example
`DEGOPLOY_LISTEN`, `DEGOPLOY_BASE_URL` or `DEGOPLOY_OIDC_CLIENT_SECRET`, and `${VAR}` references in the
file are read from the environment. Secrets belong in `/etc/degoploy/degoploy.env`, which systemd loads.
degoploy never passes `DEGOPLOY_*` variables on to deploy scripts.

The **SSO** and **Email** pages save their settings back into this file, keeping comments, and apply them
without a restart. Values set through environment variables are shown read-only there.

## Single sign-on with Casdoor

1. **In Casdoor**, create an *Application* in your organization:
   - *Redirect URLs*: `https://<degoploy host>/auth/oidc/callback`. The **SSO** page in degoploy shows
     the exact value.
   - *Grant types*: `Authorization Code`.
   - *Token format*: `JWT`. Minimal formats such as `JWT-Standard` may leave out the groups. degoploy also
     reads the userinfo endpoint, but `JWT` is the safest choice.
   - Note the *Client ID* and *Client secret*.
2. Create groups, for example `degoploy-admins` and `devops`, and add people to them.
3. **In degoploy**, open **SSO** as an admin, enter the issuer (the Casdoor origin, without a path), the
   client ID and secret, and map groups to roles, for example `my-org/degoploy-admins` → admin and
   `my-org/devops` → deployer, with a fallback role for everyone else or none to reject them. Then
   **Test sign-in** signs in once without starting a session and shows the groups Casdoor actually sends
   and the role the person would get.

The same settings can live in `degoploy.yaml` under `oidc:` (see the example file). Good to know:

- SSO accounts are linked by `issuer + sub`, never by username or email. A Casdoor user therefore cannot
  take over a local account with the same name; a new account gets a suffix such as `dewi-2`.
- Keep at least one local admin as a fallback in case Casdoor is down. After that you may turn off
  password sign-in to require SSO.
- Signing out of degoploy ends the degoploy session only, not the Casdoor session.

## Development

Requirements: Go 1.27+ (see `go.mod`), Node.js 24+ with npm, and `make`.

```bash
make dev     # API on 127.0.0.1:8090 (degoploy.dev.yaml) and Vite on 0.0.0.0:5173,
             # which proxies /api, /auth, /ws and /hook to the API
make test    # go test ./...
make check   # go vet, svelte-check, prettier and eslint
make types   # regenerate web/src/lib/api/types.gen.ts from internal/api/types.go
```

Create a first user with `go run ./cmd/degoploy user create --config degoploy.dev.yaml --username admin`,
then open http://localhost:5173. To try SSO in development, `redirect_url` must use the dev server's
address, for example `http://localhost:5173/auth/oidc/callback`; Vite forwards `/auth` to the API.

### Building

```bash
make build   # tygo, the web UI, then bin/degoploy (static, CGO_ENABLED=0)
make dist    # release/degoploy-linux-{amd64,arm64}.tar.gz and SHA256SUMS
```

Without `make`:

```bash
go tool tygo generate
(cd web && npm ci && npm run build)      # writes internal/web/dist
CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o bin/degoploy ./cmd/degoploy
```

### Releases

Pushing a version tag runs [.github/workflows/release.yml](.github/workflows/release.yml). It builds the UI
and both binaries, runs the checks and tests, and publishes the tarballs and `SHA256SUMS` as a GitHub
release, where `install.sh` and `degoploy update` download them. Tags with a hyphen (`v1.1.0-rc.1`)
become pre-releases. Releases are currently built on
[irsyadulibad/degoploy](https://github.com/irsyadulibad/degoploy), because GitHub Actions is turned off
for the Degovan organization.

```bash
git tag v1.0.1 && git push fork v1.0.1   # fork: git@github.com:irsyadulibad/degoploy.git
```

## Layout

```
cmd/degoploy/        CLI: serve, setup, update, status, user, node, php, backup, config check
internal/admin/      admin socket API for the CLI, with an in-process fallback
internal/api/        JSON API (/api/v1), webhooks (/hook), WebSocket (/ws) and OIDC (/auth)
internal/audit/      audit log entries
internal/auth/       passwords, sessions, CSRF, roles and OIDC (oidctest: a fake IdP for tests)
internal/config/     YAML and environment settings, written back by the SSO and Email pages
internal/deploy/     per-project queue and runner (git, script, log)
internal/gitops/     git and ssh settings for each project
internal/mail/       SMTP client for invitations (smtptest: a fake server for tests)
internal/toolchain/  runtimes (Node.js, PHP): version ranges, downloads, system versions, picking one
internal/project/    projects, deploy keys and scripts
internal/realtime/   WebSocket hub with topics
internal/secrets/    encryption of project secrets and sealed config values
internal/server/     router, middleware and shutdown
internal/sshkey/     deploy key generation
internal/store/      SQLite (modernc, no CGO) with embedded migrations
internal/trigger/    glob matching and trigger rules
internal/webhook/    HMAC checks and GitHub/Gitea payloads
internal/web/        the embedded web UI (built from web/)
deploy/              systemd unit and config template, embedded for degoploy setup
docs/                installation (Indonesian), the user guide and the implementation plan
install.sh           one-line installer: downloads a release, checks it, runs degoploy setup
web/                 SvelteKit with Svelte 5, Tailwind v4 and shadcn-svelte
```
