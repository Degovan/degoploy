#!/usr/bin/env bash
# Install or upgrade degoploy. Run as root from an extracted release directory:
#
#   sudo ./install.sh [--listen 127.0.0.1:8090] [--base-url https://deploy.example.com]
#                     [--admin USERNAME] [--no-start] [--binary PATH]
#
# Re-running it upgrades the binary and keeps the existing configuration and data.
set -euo pipefail

PREFIX=/usr/local/bin
CONF_DIR=/etc/degoploy
DATA_DIR=/var/lib/degoploy
SERVICE_USER=degoploy
UNIT=/etc/systemd/system/degoploy.service

LISTEN=127.0.0.1:8090
BASE_URL=
ADMIN=
NO_START=0
HERE=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
BINARY=$HERE/degoploy

usage() { sed -n '2,8p' "$0" | sed 's/^# \{0,1\}//'; exit "${1:-0}"; }
log() { printf '\033[1;36m==>\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33mwarning:\033[0m %s\n' "$*" >&2; }
die() { printf '\033[1;31merror:\033[0m %s\n' "$*" >&2; exit 1; }

while [ $# -gt 0 ]; do
	case "$1" in
	--listen) LISTEN=${2:?--listen needs a value}; shift 2 ;;
	--base-url) BASE_URL=${2:?--base-url needs a value}; shift 2 ;;
	--admin) ADMIN=${2:?--admin needs a username}; shift 2 ;;
	--binary) BINARY=${2:?--binary needs a path}; shift 2 ;;
	--no-start) NO_START=1; shift ;;
	-h | --help) usage ;;
	*) warn "unknown option $1"; usage 2 ;;
	esac
done

[ "$(id -u)" -eq 0 ] || die "run as root (sudo $0 …)"
[ -x "$BINARY" ] || die "degoploy binary not found at $BINARY (use --binary PATH)"
"$BINARY" version >/dev/null || die "$BINARY does not run on this machine (wrong architecture?)"
command -v git >/dev/null || warn "git is not installed; deployments need it (apt install git)"
command -v bash >/dev/null || die "bash is required"

HAVE_SYSTEMD=0
[ -d /run/systemd/system ] && HAVE_SYSTEMD=1
WAS_ACTIVE=0
if [ $HAVE_SYSTEMD = 1 ] && systemctl is-active --quiet degoploy 2>/dev/null; then
	WAS_ACTIVE=1
fi

# --- service account --------------------------------------------------------
if ! id "$SERVICE_USER" >/dev/null 2>&1; then
	log "Creating system user $SERVICE_USER"
	NOLOGIN=$(command -v nologin || echo /usr/sbin/nologin)
	useradd --system --user-group --home-dir "$DATA_DIR" --shell "$NOLOGIN" "$SERVICE_USER"
fi
install -d -m 0700 -o "$SERVICE_USER" -g "$SERVICE_USER" "$DATA_DIR"

# --- binary (atomic replace, so a running instance keeps working until restart)
log "Installing $("$BINARY" version) to $PREFIX/degoploy"
install -d "$PREFIX"
install -m 0755 "$BINARY" "$PREFIX/.degoploy.new"
mv -f "$PREFIX/.degoploy.new" "$PREFIX/degoploy"

# --- configuration (never overwritten) ----------------------------------------
install -d -m 0750 -o root -g "$SERVICE_USER" "$CONF_DIR"
if [ ! -f "$CONF_DIR/degoploy.yaml" ]; then
	log "Writing $CONF_DIR/degoploy.yaml"
	cat >"$CONF_DIR/degoploy.yaml" <<EOF
# degoploy configuration. Values like \${VAR} are read from the environment
# (see $CONF_DIR/degoploy.env). Check it with: degoploy config check --config $CONF_DIR/degoploy.yaml
listen: "$LISTEN"
data_dir: $DATA_DIR
# Public URL behind your reverse proxy; https:// enables Secure cookies.
base_url: "$BASE_URL"
max_parallel: 2
# Proxies whose X-Forwarded-For header is trusted (the reverse proxy on this host).
trusted_proxies: [127.0.0.1/32, "::1/128"]
session_ttl: 168h
local_login: true

oidc:
  enabled: false
  provider_name: Casdoor
  issuer: https://sso.example.com
  client_id: degoploy
  client_secret: \${DEGOPLOY_OIDC_SECRET}
  redirect_url: ${BASE_URL:-https://deploy.example.com}/auth/oidc/callback
  scopes: [openid, profile, email]
  groups_claim: groups
  role_mapping:
    "my-org/degoploy-admins": admin
    "my-org/devops": deployer
  sync_role_on_login: true
EOF
	chown root:"$SERVICE_USER" "$CONF_DIR/degoploy.yaml"
	chmod 0640 "$CONF_DIR/degoploy.yaml"
else
	log "Keeping existing $CONF_DIR/degoploy.yaml"
fi
if [ ! -f "$CONF_DIR/degoploy.env" ]; then
	cat >"$CONF_DIR/degoploy.env" <<'EOF'
# Secrets for degoploy, loaded by systemd (EnvironmentFile). Example:
# DEGOPLOY_OIDC_SECRET=change-me
EOF
	chown root:"$SERVICE_USER" "$CONF_DIR/degoploy.env"
	chmod 0640 "$CONF_DIR/degoploy.env"
fi

as_service_user() {
	if command -v runuser >/dev/null; then
		runuser -u "$SERVICE_USER" -- "$@"
	else
		su -s /bin/sh "$SERVICE_USER" -c "$(printf '%q ' "$@")"
	fi
}

log "Checking configuration"
(
	set -a
	# shellcheck disable=SC1091
	. "$CONF_DIR/degoploy.env"
	set +a
	cd "$DATA_DIR"
	as_service_user "$PREFIX/degoploy" config check --config "$CONF_DIR/degoploy.yaml"
) || die "configuration check failed; fix $CONF_DIR/degoploy.yaml and re-run"

if [ -n "$ADMIN" ]; then
	log "Creating admin user $ADMIN (you will be asked for a password)"
	(cd "$DATA_DIR" && as_service_user "$PREFIX/degoploy" user create --config "$CONF_DIR/degoploy.yaml" --username "$ADMIN" --role admin) ||
		warn "admin user not created (does it already exist?)"
fi

# --- systemd -----------------------------------------------------------------
if [ $HAVE_SYSTEMD = 1 ]; then
	if [ -f "$UNIT" ] && ! cmp -s "$HERE/degoploy.service" "$UNIT"; then
		cp "$UNIT" "$UNIT.bak"
		warn "replaced $UNIT (previous version saved as $UNIT.bak; prefer drop-ins in $UNIT.d/)"
	fi
	install -m 0644 "$HERE/degoploy.service" "$UNIT"
	systemctl daemon-reload
	systemctl enable degoploy >/dev/null
	if [ $WAS_ACTIVE = 1 ]; then
		log "Restarting degoploy (running deployments get up to 60s to finish)"
		systemctl restart degoploy
	elif [ $NO_START = 0 ]; then
		log "Starting degoploy"
		systemctl start degoploy
	fi
	[ $NO_START = 1 ] && [ $WAS_ACTIVE = 0 ] || systemctl --no-pager --lines=0 status degoploy || true
else
	warn "systemd not detected; start degoploy yourself:"
	echo "    runuser -u $SERVICE_USER -- $PREFIX/degoploy serve --config $CONF_DIR/degoploy.yaml"
fi

cat <<EOF

degoploy is installed.
  config:   $CONF_DIR/degoploy.yaml   (secrets: $CONF_DIR/degoploy.env)
  data:     $DATA_DIR
  listen:   $(grep -m1 '^listen:' "$CONF_DIR/degoploy.yaml" | sed 's/^listen: *//; s/"//g')
  logs:     journalctl -u degoploy -f

Next steps:
  - Create an admin (if you have none yet):
      sudo -u $SERVICE_USER $PREFIX/degoploy user create --config $CONF_DIR/degoploy.yaml --username admin
  - Put a reverse proxy with HTTPS in front and set base_url (see INSTALL.md).
  - Give $SERVICE_USER write access to each project's working directory.
EOF
