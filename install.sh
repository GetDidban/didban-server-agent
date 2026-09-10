#!/usr/bin/env bash
set -euo pipefail

repository="GetDidban/didban-server-agent"
install_path="/usr/local/bin/didban-agent"
environment_path="/etc/didban-agent.env"
service_path="/etc/systemd/system/didban-agent.service"
api_key="${DIDBAN_API_KEY:-}"
server_url="${DIDBAN_WS_URL:-wss://api.getdidban.ir/api/v1/live}"
agent_name="${DIDBAN_AGENT_NAME:-$(hostname)}"
agent_id="${DIDBAN_AGENT_ID:-}"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --api-key) api_key="${2:-}"; shift 2 ;;
    --server) server_url="${2:-}"; shift 2 ;;
    --name) agent_name="${2:-}"; shift 2 ;;
    --id) agent_id="${2:-}"; shift 2 ;;
    *) echo "Unknown option: $1" >&2; exit 2 ;;
  esac
done

if [[ ${EUID} -ne 0 ]]; then
  echo "Run this installer as root (for example: curl ... | sudo bash -s -- ...)." >&2
  exit 1
fi
if [[ -z "${api_key}" ]]; then
  echo "--api-key is required." >&2
  exit 2
fi
if [[ "${api_key}${server_url}${agent_name}${agent_id}" == *$'\n'* ]]; then
  echo "Configuration values cannot contain newlines." >&2
  exit 2
fi

case "$(uname -m)" in
  x86_64|amd64) architecture="amd64" ;;
  aarch64|arm64) architecture="arm64" ;;
  *) echo "Unsupported architecture: $(uname -m)" >&2; exit 1 ;;
esac

for command in curl sha256sum systemctl useradd; do
  command -v "${command}" >/dev/null || { echo "${command} is required." >&2; exit 1; }
done

temporary_directory="$(mktemp -d)"
trap 'rm -rf -- "${temporary_directory}"' EXIT
asset="didban-agent-linux-${architecture}"
release_url="https://github.com/${repository}/releases/latest/download"
if ! curl --proto '=https' --tlsv1.2 -fsSL "${release_url}/${asset}" -o "${temporary_directory}/${asset}"; then
  release_url="https://raw.githubusercontent.com/${repository}/master/dist"
  curl --proto '=https' --tlsv1.2 -fsSL "${release_url}/${asset}" -o "${temporary_directory}/${asset}"
fi
curl --proto '=https' --tlsv1.2 -fsSL "${release_url}/checksums.txt" -o "${temporary_directory}/checksums.txt"
(
  cd "${temporary_directory}"
  grep "  ${asset}$" checksums.txt | sha256sum --check --status -
)

install -m 0755 "${temporary_directory}/${asset}" "${install_path}"
if ! id didban-agent >/dev/null 2>&1; then
  useradd --system --user-group --no-create-home --shell /usr/sbin/nologin didban-agent
fi

escape_environment_value() {
  local value="$1"
  value="${value//\\/\\\\}"
  value="${value//\"/\\\"}"
  printf '"%s"' "${value}"
}

{
  printf 'DIDBAN_API_KEY='; escape_environment_value "${api_key}"; printf '\n'
  printf 'DIDBAN_WS_URL='; escape_environment_value "${server_url}"; printf '\n'
  printf 'DIDBAN_AGENT_NAME='; escape_environment_value "${agent_name}"; printf '\n'
  if [[ -n "${agent_id}" ]]; then
    printf 'DIDBAN_AGENT_ID='; escape_environment_value "${agent_id}"; printf '\n'
  fi
} > "${environment_path}"
chmod 0600 "${environment_path}"

cat > "${service_path}" <<'UNIT'
[Unit]
Description=Didban server monitoring agent
Documentation=https://github.com/GetDidban/didban-server-agent
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=didban-agent
Group=didban-agent
EnvironmentFile=/etc/didban-agent.env
ExecStart=/usr/local/bin/didban-agent
Restart=always
RestartSec=5
NoNewPrivileges=true
PrivateTmp=true
ProtectHome=true
ProtectSystem=strict
ProtectControlGroups=true
ProtectKernelModules=true
ProtectKernelTunables=true
RestrictSUIDSGID=true

[Install]
WantedBy=multi-user.target
UNIT

systemctl daemon-reload
systemctl enable --now didban-agent.service
systemctl restart didban-agent.service
echo "Didban Agent is installed and running."
echo "Status: systemctl status didban-agent --no-pager"
echo "Logs:   journalctl -u didban-agent -f"
