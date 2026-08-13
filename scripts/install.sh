#!/usr/bin/env bash
set -euo pipefail

# GitHub repository is intentionally configurable until the first public release.
REPOSITORY="${MIKROTUNNEL_REPOSITORY:-amirarzideh/MikroTunnel}"
INSTALL_DIR="/usr/local/bin"
CONFIG_DIR="/etc/mikrotunnel"
DATA_DIR="/var/lib/mikrotunnel"

if [[ "${EUID}" -ne 0 ]]; then
  echo "Run as root: curl -fsSL ... | sudo bash" >&2
  exit 1
fi
if [[ ! -f /etc/os-release ]] || ! grep -qi '^ID=ubuntu' /etc/os-release; then
  echo "MikroTunnel currently supports Ubuntu only." >&2
  exit 1
fi

arch="$(dpkg --print-architecture)"
case "${arch}" in
  amd64) asset_arch="amd64" ;;
  arm64) asset_arch="arm64" ;;
  *) echo "Unsupported architecture: ${arch}" >&2; exit 1 ;;
esac

if ! command -v curl >/dev/null; then
  apt-get update -y
  apt-get install -y curl ca-certificates
fi

release_url="https://github.com/${REPOSITORY}/releases/latest/download/mikrotunnel-linux-${asset_arch}.tar.gz"

curl_headers=()
if [[ -n "${GITHUB_TOKEN:-}" ]]; then
  curl_headers=(-H "Authorization: Bearer ${GITHUB_TOKEN}")
fi

tmp_dir="$(mktemp -d)"
trap 'rm -rf "${tmp_dir}"' EXIT
curl --fail --location --retry 3 "${curl_headers[@]}" --output "${tmp_dir}/release.tar.gz" "${release_url}"
tar -xzf "${tmp_dir}/release.tar.gz" -C "${tmp_dir}"
install -m 0755 "${tmp_dir}/mikrotunnel" "${INSTALL_DIR}/mikrotunnel"
ln -sfn "${INSTALL_DIR}/mikrotunnel" "${INSTALL_DIR}/mikrotun"
install -d -m 0755 /usr/local/lib/mikrotunnel
install -m 0755 "${tmp_dir}/setup-https.sh" /usr/local/lib/mikrotunnel/setup-https.sh

install -d -m 0750 "${CONFIG_DIR}" "${DATA_DIR}"
if [[ ! -f "${CONFIG_DIR}/config.yaml" ]]; then
  cat > "${CONFIG_DIR}/config.yaml" <<'EOF'
server:
  listen_address: 127.0.0.1:8787
storage:
  database_path: /var/lib/mikrotunnel/mikrotunnel.db
security:
  bootstrap_key_file: /var/lib/mikrotunnel/bootstrap-api-key.txt
network:
  reconcile_interval: 20s
EOF
  chmod 0640 "${CONFIG_DIR}/config.yaml"
fi

install -m 0644 "${tmp_dir}/mikrotunnel.service" /etc/systemd/system/mikrotunnel.service
systemctl daemon-reload
systemctl enable --now mikrotunnel.service
sleep 1
systemctl --quiet is-active mikrotunnel.service

echo "MikroTunnel is running on 127.0.0.1:8787."
if [[ "${MIKROTUNNEL_SKIP_HTTPS:-0}" != "1" && ! -f /etc/caddy/Caddyfile.d/mikrotunnel.caddy ]]; then
  echo "Secure HTTPS setup is required before remote access is enabled."
  /usr/local/lib/mikrotunnel/setup-https.sh
else
  echo "Secure remote access is already configured, or was explicitly skipped."
fi
if [[ -f "${DATA_DIR}/bootstrap-api-key.txt" ]]; then
  echo "API endpoint: http://127.0.0.1:8787/api/v1"
  echo "Bootstrap API key (shown once):"
  cat "${DATA_DIR}/bootstrap-api-key.txt"
  echo "Copy this key now, then securely remove ${DATA_DIR}/bootstrap-api-key.txt."
fi
