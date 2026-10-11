#!/bin/bash
set -u
trap 'exit 0' EXIT
MANAGER="43.204.226.135"
GROUP="ProductionAwsMachines"
VERSION="4.14.5-1"
BOOTLOG="/var/log/wazuh-eb-bootstrap.log"
log(){ echo "$(date -u) $*" >> "$BOOTLOG"; }
log "=== postdeploy wazuh bootstrap start ==="

if ! rpm -q wazuh-agent >/dev/null 2>&1; then
    log "installing agent $VERSION"
    rpm --import https://packages.wazuh.com/key/GPG-KEY-WAZUH >> "$BOOTLOG" 2>&1
    cat > /etc/yum.repos.d/wazuh.repo << 'REPO'
[wazuh]
gpgcheck=1
gpgkey=https://packages.wazuh.com/key/GPG-KEY-WAZUH
enabled=1
name=EL-$releasever - Wazuh
baseurl=https://packages.wazuh.com/4.x/yum/
protect=1
REPO
    WAZUH_MANAGER="$MANAGER" WAZUH_AGENT_GROUP="$GROUP" \
        yum install -y "wazuh-agent-$VERSION" >> "$BOOTLOG" 2>&1
else
    log "agent already installed; skipping install"
fi


cat > /usr/local/sbin/wazuh-autoenroll.sh << 'ENROLL'
#!/bin/bash
set -uo pipefail
MANAGER="43.204.226.135"
GROUP="ProductionAwsMachines"
KEYS="/var/ossec/etc/client.keys"
LOG="/var/log/wazuh-autoenroll.log"
echo "$(date -u) --- autoenroll start ---" >> "$LOG"
if [ -s "$KEYS" ]; then
    TOKEN=$(curl -sf -X PUT "http://169.254.169.254/latest/api/token" \
        -H "X-aws-ec2-metadata-token-ttl-seconds: 300")
    PRIV_IP=$(curl -sf -H "X-aws-ec2-metadata-token: $TOKEN" \
        "http://169.254.169.254/latest/meta-data/local-ipv4")
    EXPECTED="eb-${PRIV_IP//./-}"
    STORED=$(awk '{print $2}' "$KEYS" 2>/dev/null | head -n1)
    if [ -n "${PRIV_IP:-}" ] && [ "$STORED" = "$EXPECTED" ]; then
        echo "$(date -u) own key ($STORED), skip enroll, start agent" >> "$LOG"
        systemctl start wazuh-agent
        exit 0
    fi
    echo "$(date -u) inherited/stale key ($STORED != $EXPECTED), wiping, will re-enroll" >> "$LOG"
    rm -f "$KEYS"
fi
TOKEN=$(curl -sf -X PUT "http://169.254.169.254/latest/api/token" \
    -H "X-aws-ec2-metadata-token-ttl-seconds: 300")
PRIV_IP=$(curl -sf -H "X-aws-ec2-metadata-token: $TOKEN" \
    "http://169.254.169.254/latest/meta-data/local-ipv4")
if [ -z "${PRIV_IP:-}" ]; then
    echo "$(date -u) ERROR: no IMDS private IP; abort" >> "$LOG"; exit 1
fi
AGENT_NAME="eb-${PRIV_IP//./-}"
echo "$(date -u) enrolling as ${AGENT_NAME} -> ${MANAGER}" >> "$LOG"
/var/ossec/bin/agent-auth -m "$MANAGER" -A "$AGENT_NAME" -G "$GROUP" >> "$LOG" 2>&1
if [ -s "$KEYS" ]; then
    echo "$(date -u) enrolled OK, starting agent" >> "$LOG"
    systemctl start wazuh-agent
else
    echo "$(date -u) ERROR: enrollment produced no key" >> "$LOG"; exit 1
fi
ENROLL
chmod +x /usr/local/sbin/wazuh-autoenroll.sh


cat > /etc/systemd/system/wazuh-autoenroll.service << 'UNIT'
[Unit]
Description=Wazuh agent auto-enrollment (self-naming from IMDS private IP)
After=network-online.target
Wants=network-online.target

[Service]
Type=oneshot
ExecStart=/usr/local/sbin/wazuh-autoenroll.sh
RemainAfterExit=yes

[Install]
WantedBy=multi-user.target
UNIT


systemctl daemon-reload >> "$BOOTLOG" 2>&1
systemctl disable wazuh-agent >> "$BOOTLOG" 2>&1
systemctl enable wazuh-autoenroll.service >> "$BOOTLOG" 2>&1
systemctl restart wazuh-autoenroll.service >> "$BOOTLOG" 2>&1

log "=== postdeploy wazuh bootstrap done ==="
exit 0