#!/bin/sh
# Install or upgrade degoploy from an extracted release directory:
#
#   sudo ./install.sh [--listen 127.0.0.1:8090] [--base-url https://deploy.example.com]
#                     [--admin USERNAME] [--operator USER | --no-operator]
#                     [--no-start] [--yes]
#
# It is the same as running ./degoploy setup, which the binary also does by
# itself when started in a terminal on a server without degoploy.
set -eu
HERE=$(cd "$(dirname "$0")" && pwd)
exec "$HERE/degoploy" setup "$@"
