#!/usr/bin/env bash
set -euo pipefail

readonly CADDYFILE="/etc/caddy/Caddyfile"
readonly INCLUDE_DIR="/etc/caddy/Caddyfile.d"
readonly SITE_FILE="${INCLUDE_DIR}/mikrotunnel.caddy"
readonly ACME_ROOT="/var/lib/caddy/mikrotunnel-acme"
readonly TLS_DIR="/etc/mikrotunnel/tls"

if [[ "${EUID}" -ne 0 ]]; then
  echo "Run as root: sudo mikrotun setup https" >&2
  exit 1
fi

read_value() {
  local prompt="$1" default="${2:-}" value
  [[ -r /dev/tty ]] || { echo "An interactive terminal is required." >&2; exit 1; }
  if [[ -n "${default}" ]]; then
    read -r -p "${prompt} [${default}]: " value < /dev/tty
    printf '%s' "${value:-${default}}"
  else
    read -r -p "${prompt}: " value < /dev/tty
    printf '%s' "${value}"
  fi
}

valid_host() { [[ "$1" =~ ^[A-Za-z0-9.-]+$ ]] && [[ "$1" != *..* ]] && [[ "$1" != .* ]] && [[ "$1" != *- ]]; }
valid_email() { [[ "$1" =~ ^[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}$ ]]; }
is_ipv4() { [[ "$1" =~ ^([0-9]{1,3}\.){3}[0-9]{1,3}$ ]]; }

public_ip="$(curl -4fsS --max-time 8 https://api.ipify.org 2>/dev/null || true)"
echo "MikroTunnel secure remote access"
echo "Enter a domain name for standard automatic HTTPS, or keep the public-IP default."
host="$(read_value 'Public hostname or IP address' "${public_ip}")"
echo "Email is optional; it is used only for certificate expiry notices."
email="$(read_value 'Email for certificate expiry notices (optional)' 'skip')"
[[ "${email}" == "skip" ]] && email=""
valid_host "${host}" || { echo "Invalid hostname or IP address." >&2; exit 1; }
[[ -z "${email}" ]] || valid_email "${email}" || { echo "Invalid email." >&2; exit 1; }

export DEBIAN_FRONTEND=noninteractive
apt-get update -y
apt-get install -y caddy ca-certificates
install -d -m 0755 "${INCLUDE_DIR}"
install -d -m 0755 -o caddy -g caddy "${ACME_ROOT}"
if ! grep -Fq 'import /etc/caddy/Caddyfile.d/*' "${CADDYFILE}"; then
  cp "${CADDYFILE}" "${CADDYFILE}.mikrotunnel-backup-$(date +%s)"
  printf '\n# MikroTunnel managed sites\nimport /etc/caddy/Caddyfile.d/*\n' >> "${CADDYFILE}"
fi

common_headers='header {
    Strict-Transport-Security "max-age=31536000; includeSubDomains"
    X-Content-Type-Options "nosniff"
    X-Frame-Options "DENY"
    Referrer-Policy "same-origin"
  }'

if ! is_ipv4 "${host}"; then
  tls_config=""
  if [[ -n "${email}" ]]; then
    tls_config=$(cat <<EOF
  tls {
    issuer acme {
      email ${email}
    }
  }
EOF
)
  fi
  cat > "${SITE_FILE}" <<EOF
${host} {
${tls_config}
  encode zstd gzip
  ${common_headers}
  reverse_proxy 127.0.0.1:8787
}
EOF
  caddy validate --config "${CADDYFILE}" --adapter caddyfile
  systemctl enable --now caddy
  systemctl restart caddy
  systemctl --quiet is-active caddy
  echo "Secure access is ready: https://${host}"
  echo "Caddy will obtain and renew the certificate automatically."
  exit 0
fi

if ! command -v snap >/dev/null 2>&1; then
  apt-get install -y snapd
fi
snap install --classic certbot || snap refresh certbot
ln -sfn /snap/bin/certbot /usr/local/bin/certbot
if ! certbot --help all | grep -q -- '--ip-address'; then
  echo "Installed Certbot does not support automatic IP certificates." >&2
  exit 1
fi

cat > "${SITE_FILE}" <<EOF
http://${host} {
  handle /.well-known/acme-challenge/* {
    root * ${ACME_ROOT}
    file_server
  }
  redir https://${host}{uri}
}

https://${host} {
  tls internal
  handle /.well-known/acme-challenge/* {
    root * ${ACME_ROOT}
    file_server
  }
  reverse_proxy 127.0.0.1:8787
}
EOF
caddy validate --config "${CADDYFILE}" --adapter caddyfile
systemctl enable --now caddy
systemctl restart caddy

certbot_args=(certonly --non-interactive --agree-tos --preferred-profile shortlived --webroot --webroot-path "${ACME_ROOT}" --ip-address "${host}")
if [[ -n "${email}" ]]; then
  certbot_args+=(--email "${email}")
else
  certbot_args+=(--register-unsafely-without-email)
fi
certbot "${certbot_args[@]}"

install -d -m 0750 -o root -g caddy "${TLS_DIR}"
cat > /usr/local/lib/mikrotunnel/refresh-ip-certificate.sh <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
lineage="$1"
install -m 0644 -o root -g caddy "${lineage}/fullchain.pem" /etc/mikrotunnel/tls/certificate.pem
install -m 0640 -o root -g caddy "${lineage}/privkey.pem" /etc/mikrotunnel/tls/private-key.pem
systemctl reload caddy
EOF
chmod 0750 /usr/local/lib/mikrotunnel/refresh-ip-certificate.sh
/usr/local/lib/mikrotunnel/refresh-ip-certificate.sh "/etc/letsencrypt/live/${host}"
cat > /etc/systemd/system/mikrotunnel-ip-certificate-renew.service <<'EOF'
[Unit]
Description=Renew MikroTunnel public-IP certificate
After=network-online.target

[Service]
Type=oneshot
ExecStart=/usr/local/bin/certbot renew --quiet --deploy-hook '/usr/local/lib/mikrotunnel/refresh-ip-certificate.sh "$RENEWED_LINEAGE"'
EOF
cat > /etc/systemd/system/mikrotunnel-ip-certificate-renew.timer <<'EOF'
[Unit]
Description=Renew MikroTunnel public-IP certificate twice daily

[Timer]
OnCalendar=*-*-* 00,12:00:00
Persistent=true

[Install]
WantedBy=timers.target
EOF
systemctl daemon-reload
systemctl enable --now mikrotunnel-ip-certificate-renew.timer

cat > "${SITE_FILE}" <<EOF
http://${host} {
  handle /.well-known/acme-challenge/* {
    root * ${ACME_ROOT}
    file_server
  }
  redir https://${host}{uri}
}

https://${host} {
  tls ${TLS_DIR}/certificate.pem ${TLS_DIR}/private-key.pem
  encode zstd gzip
  ${common_headers}
  reverse_proxy 127.0.0.1:8787
}
EOF
caddy validate --config "${CADDYFILE}" --adapter caddyfile
systemctl restart caddy
systemctl --quiet is-active caddy

echo "Secure access is ready: https://${host}"
echo "The IP certificate is short-lived; Certbot renews it automatically."
echo "Ensure your firewall/security group allows TCP 80 and 443."
