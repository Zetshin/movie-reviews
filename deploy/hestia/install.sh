#!/bin/bash
# One-time (and idempotent) HestiaCP setup for MovieClub TH. Run as root.
#
#   install.sh <hestia-user> <domain>
#   e.g. install.sh Arruto movieclubth.com
#
# - installs the docker-movieclub nginx proxy template
# - applies it to the domain (Hestia rebuilds the vhost and reloads nginx)
# - enables AWStats for the domain so web statistics show in the panel
#
# Bandwidth in the Hestia panel comes from the server-level .bytes log the
# template writes; Hestia's nightly "traffic" job adds it to U_BANDWIDTH.
set -euo pipefail

USER_NAME="${1:?usage: install.sh <hestia-user> <domain>}"
DOMAIN="${2:?usage: install.sh <hestia-user> <domain>}"
HESTIA=/usr/local/hestia
HERE="$(cd "$(dirname "$0")" && pwd)"
TPL_DIR="$HESTIA/data/templates/web/nginx"

[ "$(id -u)" -eq 0 ] || { echo "run as root" >&2; exit 1; }
[ -x "$HESTIA/bin/v-change-web-domain-proxy-tpl" ] || { echo "HestiaCP not found" >&2; exit 1; }

install -m 644 "$HERE/docker-movieclub.tpl"  "$TPL_DIR/docker-movieclub.tpl"
install -m 644 "$HERE/docker-movieclub.stpl" "$TPL_DIR/docker-movieclub.stpl"
echo "Installed proxy template docker-movieclub"

"$HESTIA/bin/v-change-web-domain-proxy-tpl" "$USER_NAME" "$DOMAIN" docker-movieclub
echo "Applied proxy template to $DOMAIN"

# AWStats web statistics (panel: Web -> domain -> statistics). Ignore the
# error Hestia returns when stats are already enabled.
"$HESTIA/bin/v-add-web-domain-stats" "$USER_NAME" "$DOMAIN" awstats 2>/dev/null || true

nginx -t
echo "Done."
