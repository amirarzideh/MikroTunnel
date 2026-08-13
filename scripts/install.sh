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
  api_key_file: /etc/mikrotunnel/api-key.txt
  dashboard_username: admin
  dashboard_password: admin123
network:
  reconcile_interval: 20s
EOF
  chmod 0640 "${CONFIG_DIR}/config.yaml"
fi
if ! grep -q '^  api_key_file:' "${CONFIG_DIR}/config.yaml"; then
  sed -i 's|^  bootstrap_key_file:.*|  api_key_file: /etc/mikrotunnel/api-key.txt\n  dashboard_username: admin\n  dashboard_password: admin123|' "${CONFIG_DIR}/config.yaml"
fi
if [[ ! -s "${CONFIG_DIR}/api-key.txt" ]]; then
  umask 077
  printf 'mt_%s\n' "$(od -An -N 32 -tx1 /dev/urandom | tr -d ' \n')" > "${CONFIG_DIR}/api-key.txt"
fi
chmod 0600 "${CONFIG_DIR}/api-key.txt"

install -m 0644 "${tmp_dir}/mikrotunnel.service" /etc/systemd/system/mikrotunnel.service
systemctl daemon-reload
systemctl enable --now mikrotunnel.service
sleep 1
systemctl --quiet is-active mikrotunnel.service

echo "MikroTunnel is running on 127.0.0.1:8787."
if [[ "${MIKROTUNNEL_SKIP_HTTPS:-0}" != "1" ]]; then
  echo "Secure HTTPS setup is required before remote access is enabled."
  /usr/local/lib/mikrotunnel/setup-https.sh
else
  echo "Secure remote access was explicitly skipped."
fi
echo "Dashboard login: admin / admin123"
echo "Persistent API endpoint: https://<your-server>/api/v1"
echo "Persistent API key: $(cat "${CONFIG_DIR}/api-key.txt")"
echo "Change the default dashboard password in ${CONFIG_DIR}/config.yaml, then run: sudo mikrotun service restart"
