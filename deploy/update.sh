#!/bin/bash
# Update an FFC WFC server installed by vultr_setup.sh (/opt/wfc, user wwfc) to a newer commit. Run as root:
#   curl -fsSL https://raw.githubusercontent.com/totomam/wfc-server-FFC/ccr-d95493a2-r52wk0/deploy/update.sh | bash
# BRANCH / COMMIT override what is built (default: this branch's head). Keeps config.xml, the DB, balance/, package/.
set -euo pipefail
[ "$(id -u)" = 0 ] || { echo "run as root"; exit 1; }
BRANCH=${BRANCH:-ccr-d95493a2-r52wk0}
DIR=/opt/wfc
export PATH=$PATH:/usr/local/go/bin
[ -d $DIR/.git ] || { echo "$DIR missing: run vultr_setup.sh first"; exit 1; }
W() { sudo -u wwfc -H env PATH="$PATH" "$@"; }
OLD=$(W git -C $DIR rev-parse --short HEAD)
W git -C $DIR fetch -q origin "$BRANCH"
W git -C $DIR checkout -q "${COMMIT:-FETCH_HEAD}"
(cd $DIR && W go build -o wwfc.new . && mv wwfc.new wwfc)
W mkdir -p $DIR/balance $DIR/package
systemctl restart wwfc
sleep 3
echo "== updated $OLD -> $(W git -C $DIR log -1 --format='%h %s')"
echo "service: $(systemctl is-active wwfc)   (logs: journalctl -u wwfc -f)"
echo "package: $(ls -l $DIR/package/VDEE.bin 2>/dev/null || echo 'none yet (vanilla for everyone)')"
echo "table:   $(ls -l $DIR/balance/VDEE.bin 2>/dev/null || echo 'none yet (stock table)')"
