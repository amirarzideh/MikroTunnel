#!/usr/bin/env bash
set -euo pipefail

readonly CADDYFILE="/etc/caddy/Caddyfile"
readonly INCLUDE_DIR="/etc/caddy/Caddyfile.d"
readonly SITE_FILE="${INCLUDE_DIR}/mikrotunnel.caddy"
readonly TLS_DIR="/etc/mikrotunnel/tls"

if [[ "${EUID}" -ne 0 ]]; then
  echo "Run as root: sudo mikrotun setup https" >&2
  exit 1
fi

read_value() {
  local prompt="$1" value
  [[ -r /dev/tty ]] || { echo "An interactive terminal is required." >&2; exit 1; }
  read -r -p "${prompt}: " value < /dev/tty
  printf '%s' "${value}"
}

valid_host() { [[ "$1" =~ ^[A-Za-z0-9.-]+$ ]] && [[ "$1" != *..* ]] && [[ "$1" != .* ]] && [[ "$1" != *- ]]; }
valid_email() { [[ "$1" =~ ^[^[:space:]@]+@[^[:space:]@]+\.[^[:space:]@]+$ ]]; }

echo "MikroTunnel secure remote access"
echo "1) Domain name + automatic Let's Encrypt certificate"
echo "2) IP address or domain + certificate and key you already own"
mode="$(read_value 'Choose 1 or 2')"

case "${mode}" in
  1)
    host="$(read_value 'Domain name (DNS A/AAAA must point to this server)')"
    email="$(read_value 'Email for certificate expiry notices')"
    valid_host "${host}" || { echo "Invalid domain name." >&2; exit 1; }
    valid_email "${email}" || { echo "Invalid email." >&2; exit 1; }
    site=$(cat <<EOF
${host} {
  tls {
    issuer acme {
      email ${email}
    }
  }
  encode zstd gzip
  header {
    Strict-Transport-Security "max-age=31536000; includeSubDomains"
    X-Content-Type-Options "nosniff"
    X-Frame-Options "DENY"
    Referrer-Policy "same-origin"
  }
  reverse_proxy 127.0.0.1:8787
}
EOF
)
    public_url="https://${host}"
    ;;
  2)
    host="$(read_value 'Public IP address or hostname')"
    cert="$(read_value 'Full path to PEM certificate')"
    key="$(read_value 'Full path to PEM private key')"
    valid_host "${host}" || { echo "Invalid IP address or hostname." >&2; exit 1; }
    [[ -f "${cert}" && -f "${key}" ]] || { echo "Certificate or private key file does not exist." >&2; exit 1; }
    ;;
  *) echo "Choose 1 or 2." >&2; exit 1 ;;
esac

export DEBIAN_FRONTEND=noninteractive
apt-get update -y
apt-get install -y caddy ca-certificates

install -d -m 0755 "${INCLUDE_DIR}"
if ! grep -Fq 'import /etc/caddy/Caddyfile.d/*' "${CADDYFILE}"; then
  cp "${CADDYFILE}" "${CADDYFILE}.mikrotunnel-backup-$(date +%s)"
  printf '\n# MikroTunnel managed sites\nimport /etc/caddy/Caddyfile.d/*\n' >> "${CADDYFILE}"
fi

if [[ "${mode}" == "2" ]]; then
  install -d -m 0750 -o root -g caddy "${TLS_DIR}"
  install -m 0644 -o root -g caddy "${cert}" "${TLS_DIR}/certificate.pem"
  install -m 0640 -o root -g caddy "${key}" "${TLS_DIR}/private-key.pem"
  site=$(cat <<EOF
https://${host} {
  tls ${TLS_DIR}/certificate.pem ${TLS_DIR}/private-key.pem
  encode zstd gzip
  header {
    Strict-Transport-Security "max-age=31536000"
    X-Content-Type-Options "nosniff"
    X-Frame-Options "DENY"
    Referrer-Policy "same-origin"
  }
  reverse_proxy 127.0.0.1:8787
}
EOF
)
  public_url="https://${host}"
fi

printf '%s\n' "${site}" > "${SITE_FILE}"
caddy validate --config "${CADDYFILE}" --adapter caddyfile
systemctl enable --now caddy
systemctl restart caddy
systemctl --quiet is-active caddy

echo
echo "Secure access is ready: ${public_url}"
echo "The MikroTunnel agent remains private on 127.0.0.1:8787."
echo "Ensure your firewall/security group allows TCP 80 and 443."
