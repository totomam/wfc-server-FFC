#!/bin/bash
# One-shot FFC WFC server setup for a fresh Ubuntu 24.04 VPS (Vultr etc.). Run as root:
#   curl -fsSL https://raw.githubusercontent.com/totomam/wfc-server-FFC/claude/ffc-fix03-y29slm/deploy/vultr_setup.sh | bash
# Phase 1 (no DS certs): HTTP only, port 443 stays closed. Certs: docs/online/HOSTING.md §6a in ffc-decomp.
set -euo pipefail
[ "$(id -u)" = 0 ] || { echo "run as root"; exit 1; }

BRANCH=${BRANCH:-claude/ffc-fix03-y29slm}   # release branch (FFC-DECOMP NET-17/18)
COMMIT=${COMMIT:-FETCH_HEAD}   # default: head of the release branch $BRANCH
GO_VER=1.25.0
DIR=/opt/wfc
LOCAL_IP=$(ip -4 route get 1.1.1.1 | awk '{for(i=1;i<=NF;i++) if($i=="src") print $(i+1)}')
# The NIC may carry a private/NAT address (e.g. 10.0.2.15); DNS answers must be the public IP.
IP=${PUBLIC_IP:-$(curl -4 -fsS --max-time 10 https://api.ipify.org || curl -4 -fsS --max-time 10 https://ifconfig.me)}
echo "== public IP: $IP   local IP: $LOCAL_IP"

export DEBIAN_FRONTEND=noninteractive
apt-get update -q
apt-get install -yq postgresql dnsmasq git ufw curl openssl dnsutils

# Go (Ubuntu's is too old for go.mod 1.25)
if ! /usr/local/go/bin/go version 2>/dev/null | grep -q "go$GO_VER"; then
  curl -fsSL "https://go.dev/dl/go$GO_VER.linux-amd64.tar.gz" -o /tmp/go.tgz
  rm -rf /usr/local/go && tar -C /usr/local -xzf /tmp/go.tgz && rm /tmp/go.tgz
fi
export PATH=$PATH:/usr/local/go/bin

# Firewall (SSH first so we never lock ourselves out)
ufw allow 22/tcp
for p in 80 28910 29900 29901 29920; do ufw allow $p/tcp; done
for p in 27900 27901 53; do ufw allow $p/udp; done
ufw --force enable

# Service user + Postgres
id wwfc >/dev/null 2>&1 || useradd -r -m -d /var/lib/wwfc -s /usr/sbin/nologin wwfc
if [ ! -f /root/wwfc-db.txt ]; then openssl rand -hex 16 > /root/wwfc-db.txt; chmod 600 /root/wwfc-db.txt; fi
DBPW=$(cat /root/wwfc-db.txt)
sudo -u postgres psql -tAc "SELECT 1 FROM pg_roles WHERE rolname='wwfc'" | grep -q 1 \
  || sudo -u postgres psql -c "CREATE ROLE wwfc WITH LOGIN SUPERUSER PASSWORD '$DBPW';"
sudo -u postgres psql -tAc "SELECT 1 FROM pg_database WHERE datname='wwfc'" | grep -q 1 \
  || sudo -u postgres createdb -O wwfc wwfc

# Build
[ -d $DIR/.git ] || git clone -b "$BRANCH" https://github.com/totomam/wfc-server-FFC $DIR
git -C $DIR fetch -q origin "$BRANCH" && git -C $DIR checkout -q "$COMMIT"
(cd $DIR && go build -o wwfc .)
PGPASSWORD=$DBPW psql -q -h 127.0.0.1 -U wwfc -d wwfc -f $DIR/schema.sql || true  # re-run safe-ish

# config.xml (only written once; edit by hand later)
if [ ! -f $DIR/config.xml ]; then
  sed -e 's#<gsAddress>.*<#<gsAddress>0.0.0.0<#' \
      -e 's#<nasAddress>.*<#<nasAddress>0.0.0.0<#' \
      -e 's#<nasAddressHttps>.*<#<nasAddressHttps>0.0.0.0<#' \
      -e 's#<payloadServerAddress>.*<#<payloadServerAddress><#' \
      -e "s#<username>.*<#<username>wwfc<#" \
      -e "s#<password>.*<#<password>$DBPW<#" \
      -e "s#<apiSecret>.*<#<apiSecret>$(openssl rand -hex 32)<#" \
      -e 's#<allowDefaultDolphinKeys>.*<#<allowDefaultDolphinKeys>false<#' \
      -e '/<discord>/,/<\/discord>/d' \
      $DIR/config_example.xml > $DIR/config.xml
fi
mkdir -p $DIR/balance
chown -R wwfc:wwfc $DIR && chmod 600 $DIR/config.xml

# DNS: answer ONLY nintendowifi.net (no upstream = not an open resolver)
cat > /etc/dnsmasq.d/ffc.conf <<DNS
address=/nintendowifi.net/$IP
listen-address=$LOCAL_IP
bind-interfaces
no-resolv
DNS
# Ubuntu's systemd-resolved owns 127.0.0.53:53; keep dnsmasq off loopback entirely
grep -q '^DNSMASQ_EXCEPT=' /etc/default/dnsmasq 2>/dev/null || echo 'DNSMASQ_EXCEPT="lo"' >> /etc/default/dnsmasq
systemctl enable -q dnsmasq || true
if ! systemctl restart dnsmasq; then
  echo "!! dnsmasq failed (server still installs; players can use the hosts-file method):"
  journalctl -u dnsmasq -n 15 --no-pager || true
fi

# systemd service
cat > /etc/systemd/system/wwfc.service <<UNIT
[Unit]
Description=FFC WFC server
After=network-online.target postgresql.service
Wants=network-online.target
[Service]
User=wwfc
WorkingDirectory=$DIR
ExecStart=$DIR/wwfc
Restart=on-failure
AmbientCapabilities=CAP_NET_BIND_SERVICE
[Install]
WantedBy=multi-user.target
UNIT
systemctl daemon-reload && systemctl enable -q wwfc && systemctl restart wwfc

# Nightly DB backup, 14 days kept
printf '127.0.0.1:5432:wwfc:wwfc:%s\n' "$DBPW" > /root/.pgpass && chmod 600 /root/.pgpass
cat > /etc/cron.d/wwfc-backup <<CRON
0 4 * * * root pg_dump -h 127.0.0.1 -U wwfc wwfc | gzip > /var/backups/wwfc-\$(date +\%F).sql.gz; find /var/backups -name 'wwfc-*.sql.gz' -mtime +14 -delete
CRON

sleep 3
echo
echo "== DONE"
echo "IP:      $IP"
echo "commit:  $(git -C $DIR log -1 --format='%h %s')"
echo "service: $(systemctl is-active wwfc)   (logs: journalctl -u wwfc -f)"
echo "DNS:     $(dig +short @$LOCAL_IP gpcm.gs.nintendowifi.net 2>/dev/null || echo 'install dnsutils to check')"
echo "Also add these ports to the Vultr firewall group if you use one: TCP 22,80,28910,29900,29901,29920 UDP 53,27900,27901"
