#!/bin/sh
# Install degoploy on a Linux server:
#
#   curl -fsSL https://raw.githubusercontent.com/irsyadulibad/degoploy/main/install.sh | sh
#
# It downloads the release for this machine, checks its SHA256 sum, and runs
# degoploy setup, which asks a few questions and installs the service. Run it
# again to upgrade. Options go after "sh -s --":
#
#   --version v1.2.3   install that release instead of the latest
#   --yes              take the defaults; with the other degoploy setup flags
#                      (--base-url URL, --admin NAME, ...) for scripts
#
# DEGOPLOY_DOWNLOAD_URL replaces https://github.com/irsyadulibad/degoploy/releases,
# for a mirror.
set -eu

main() {
	base=${DEGOPLOY_DOWNLOAD_URL:-https://github.com/irsyadulibad/degoploy/releases}
	version=latest
	assume_yes=0

	# Take --version; pass everything else on to degoploy setup.
	n=$#
	while [ "$n" -gt 0 ]; do
		arg=$1
		shift
		n=$((n - 1))
		case $arg in
		--version)
			[ "$n" -gt 0 ] || die "--version needs a value, such as v1.2.3"
			version=$1
			shift
			n=$((n - 1))
			;;
		--version=*) version=${arg#*=} ;;
		-h | --help)
			sed -n '2,15p' "$0" 2>/dev/null | sed 's/^# \{0,1\}//'
			exit 0
			;;
		*)
			case $arg in -y | --yes) assume_yes=1 ;; esac
			set -- "$@" "$arg"
			;;
		esac
	done

	[ "$(uname -s)" = Linux ] || die "degoploy runs on Linux; this is $(uname -s)"
	case $(uname -m) in
	x86_64 | amd64) arch=amd64 ;;
	aarch64 | arm64) arch=arm64 ;;
	*) die "no degoploy release for $(uname -m); build it from source (see README.md)" ;;
	esac

	if [ "$version" = latest ]; then
		url=$base/latest/download
	else
		url=$base/download/$version
	fi
	file=degoploy-linux-$arch.tar.gz

	tmp=$(mktemp -d)
	trap 'rm -rf "$tmp"' EXIT INT TERM
	say "Downloading degoploy ($version, linux/$arch)"
	fetch "$url/$file" "$tmp/$file" progress || die "could not download $url/$file"
	fetch "$url/SHA256SUMS" "$tmp/SHA256SUMS" || die "could not download $url/SHA256SUMS"

	want=$(awk -v f="$file" '$2 == f || $2 == "*" f { print $1 }' "$tmp/SHA256SUMS")
	[ -n "$want" ] || die "$file is not listed in SHA256SUMS"
	got=$(sha256 "$tmp/$file")
	[ "$want" = "$got" ] || die "checksum mismatch for $file; the download is damaged or was altered"

	tar -xzf "$tmp/$file" -C "$tmp"
	bin=$tmp/degoploy-linux-$arch/degoploy
	"$bin" version >/dev/null 2>&1 || die "the downloaded degoploy does not run on this machine"
	say "Verified degoploy $("$bin" version)"
	echo

	# Installing needs root. sudo keeps who ran it, and setup lets that user
	# manage degoploy without sudo afterwards.
	run=
	if [ "$(id -u)" != 0 ]; then
		command -v sudo >/dev/null 2>&1 || die "installing needs root; run this as root"
		run=sudo
		say "Installing needs root; sudo may ask for your password"
	fi

	# Piped into sh, stdin is this script: the questions read the terminal
	# instead. With --yes, stdin stays for a password piped by a script.
	if [ "$assume_yes" = 0 ] && (exec </dev/tty) 2>/dev/null; then
		$run "$bin" setup "$@" </dev/tty
	else
		$run "$bin" setup "$@"
	fi
}

say() { printf '\033[1;36m==>\033[0m %s\n' "$*"; }
die() {
	printf '\033[1;31merror:\033[0m %s\n' "$*" >&2
	exit 1
}

# fetch URL FILE [progress]: with "progress", show a progress bar when a
# terminal is there to see it.
fetch() {
	bar=0
	[ "${3:-}" = progress ] && [ -t 2 ] && bar=1
	if command -v curl >/dev/null 2>&1; then
		if [ "$bar" = 1 ]; then
			curl -fL --retry 3 --progress-bar -o "$2" "$1"
		else
			curl -fsSL --retry 3 -o "$2" "$1"
		fi
	elif command -v wget >/dev/null 2>&1; then
		if [ "$bar" = 1 ] && wget --help 2>&1 | grep -q -- --show-progress; then
			wget -q --show-progress -O "$2" "$1"
		else
			wget -q -O "$2" "$1"
		fi
	else
		die "curl or wget is needed to download degoploy"
	fi
}

sha256() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | awk '{ print $1 }'
	elif command -v shasum >/dev/null 2>&1; then
		shasum -a 256 "$1" | awk '{ print $1 }'
	else
		die "sha256sum is needed to check the download"
	fi
}

# The whole script is read before main runs, so nothing below is lost when
# setup reads stdin.
main "$@"
