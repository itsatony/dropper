#!/usr/bin/env bash
# dropper interactive setup script
# Detects container runtime, prompts for configuration, builds and starts dropper.
set -euo pipefail

# --- Constants ---
readonly DEFAULT_PORT=8080
readonly DEFAULT_ROOT_DIR="./data"
readonly DEFAULT_MAX_UPLOAD="104857600"
readonly DEFAULT_READONLY="false"
readonly SECRET_LENGTH=32
readonly CONFIG_DIR="configs"
readonly CONFIG_FILE="${CONFIG_DIR}/dropper.yaml"
readonly DOCKER_IMAGE="vaudience/dropper"
readonly DOCKER_TAG="latest"
readonly CONTAINER_NAME="dropper"

# --- Color output ---
red()    { printf '\033[0;31m%s\033[0m\n' "$*"; }
green()  { printf '\033[0;32m%s\033[0m\n' "$*"; }
yellow() { printf '\033[0;33m%s\033[0m\n' "$*"; }
bold()   { printf '\033[1m%s\033[0m\n' "$*"; }

# --- Helpers ---
generate_secret() {
    tr -dc 'a-zA-Z0-9' < /dev/urandom | head -c "${SECRET_LENGTH}" 2>/dev/null || true
}

prompt() {
    local prompt_text="$1" default_val="$2"
    local input
    printf '%s [%s]: ' "$prompt_text" "$default_val"
    read -r input
    REPLY="${input:-${default_val}}"
}

prompt_yn() {
    local prompt_text="$1" default_val="$2"
    local input
    printf '%s [%s]: ' "$prompt_text" "$default_val"
    read -r input
    input="${input:-${default_val}}"
    case "${input}" in
        [yY]|[yY][eE][sS]) REPLY="true" ;;
        *)                  REPLY="false" ;;
    esac
}

check_port() {
    local port="$1"
    if command -v ss &>/dev/null; then
        if ss -tlnp 2>/dev/null | grep -q ":${port} "; then
            return 1
        fi
    elif command -v lsof &>/dev/null; then
        if lsof -i ":${port}" &>/dev/null; then
            return 1
        fi
    fi
    return 0
}

# --- Detect container runtime ---
detect_runtime() {
    if command -v podman &>/dev/null; then
        echo "podman"
    elif command -v docker &>/dev/null; then
        echo "docker"
    else
        red "Error: neither podman nor docker found in PATH."
        red "Install one of them and try again."
        exit 1
    fi
}

# --- Main ---
main() {
    bold "=== dropper setup ==="
    echo ""

    # Detect runtime
    local runtime
    runtime="$(detect_runtime)"
    green "Container runtime: ${runtime}"
    echo ""

    # Generate default secret
    local generated_secret
    generated_secret="$(generate_secret)"

    # --- Interactive prompts ---
    local secret port root_dir max_upload readonly_mode allowed_exts

    prompt "Pre-shared secret" "${generated_secret}"
    secret="${REPLY}"

    if [ ${#secret} -lt 8 ]; then
        red "Error: secret must be at least 8 characters."
        exit 1
    fi

    prompt "Listen port" "${DEFAULT_PORT}"
    port="${REPLY}"

    if ! check_port "${port}"; then
        yellow "Warning: port ${port} appears to be in use."
        prompt "Choose a different port" "$((port + 1))"
        port="${REPLY}"
    fi

    prompt "Host directory to mount as /data" "${DEFAULT_ROOT_DIR}"
    root_dir="${REPLY}"

    prompt "Max upload size in bytes (104857600 = 100MB)" "${DEFAULT_MAX_UPLOAD}"
    max_upload="${REPLY}"

    prompt_yn "Read-only mode? (y/N)" "N"
    readonly_mode="${REPLY}"

    prompt "Allowed extensions (comma-separated, empty = all)" ""
    allowed_exts="${REPLY}"

    echo ""
    bold "--- Configuration summary ---"
    echo "  Port:         ${port}"
    echo "  Root dir:     ${root_dir}"
    echo "  Max upload:   ${max_upload} bytes"
    echo "  Read-only:    ${readonly_mode}"
    echo "  Extensions:   ${allowed_exts:-all}"
    echo ""

    # --- Create root directory if needed ---
    if [ ! -d "${root_dir}" ]; then
        yellow "Creating data directory: ${root_dir}"
        mkdir -p "${root_dir}"
    fi

    # --- Write config file ---
    mkdir -p "${CONFIG_DIR}"

    # Build allowed_extensions YAML value
    local ext_yaml="[]"
    if [ -n "${allowed_exts}" ]; then
        ext_yaml="["
        local first=true
        IFS=',' read -ra exts <<< "${allowed_exts}"
        for ext in "${exts[@]}"; do
            ext="$(echo "${ext}" | xargs)"  # trim whitespace
            if [ "${first}" = true ]; then
                ext_yaml="${ext_yaml}\"${ext}\""
                first=false
            else
                ext_yaml="${ext_yaml}, \"${ext}\""
            fi
        done
        ext_yaml="${ext_yaml}]"
    fi

    cat > "${CONFIG_FILE}" <<YAML
dropper:
  listen_port: 8080
  secret: "${secret}"
  session_ttl: "24h"
  rate_limit_login: 5
  root_dir: "/data"
  readonly: ${readonly_mode}
  max_upload_bytes: ${max_upload}
  allowed_extensions: ${ext_yaml}
  audit_log_path: "/var/log/dropper_audit.log"
  logging:
    level: "info"
    format: "json"
    output: "stdout"
    no_log_paths:
      - "/healthz"
      - "/version"
      - "/metrics"
YAML

    green "Config written to ${CONFIG_FILE}"
    echo ""

    # --- Build image ---
    bold "Building container image..."
    local git_commit git_tag build_time
    git_commit="$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")"
    git_tag="v$(grep '  version:' versions.yaml | sed 's/.*"\(.*\)"/\1/')"
    build_time="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

    ${runtime} build \
        --build-arg "GIT_COMMIT=${git_commit}" \
        --build-arg "GIT_TAG=${git_tag}" \
        --build-arg "BUILD_TIME=${build_time}" \
        -t "${DOCKER_IMAGE}:${DOCKER_TAG}" .

    green "Image built: ${DOCKER_IMAGE}:${DOCKER_TAG}"
    echo ""

    # --- Stop existing container if running ---
    ${runtime} stop "${CONTAINER_NAME}" 2>/dev/null || true
    ${runtime} rm "${CONTAINER_NAME}" 2>/dev/null || true

    # --- Start container ---
    bold "Starting dropper..."
    local abs_root_dir abs_config
    abs_root_dir="$(cd "${root_dir}" && pwd)"
    abs_config="$(cd "$(dirname "${CONFIG_FILE}")" && pwd)/$(basename "${CONFIG_FILE}")"

    ${runtime} run -d \
        --name "${CONTAINER_NAME}" \
        -p "${port}:8080" \
        -v "${abs_config}:/etc/dropper/dropper.yaml:ro" \
        -v "${abs_root_dir}:/data" \
        -e "DROPPER_SECRET=${secret}" \
        "${DOCKER_IMAGE}:${DOCKER_TAG}" \
        --config /etc/dropper/dropper.yaml

    echo ""
    green "=== dropper is running ==="
    echo ""
    bold "Access:"
    echo "  http://localhost:${port}"
    echo ""
    bold "Secret:"
    echo "  ${secret}"
    echo ""
    bold "Reverse proxy (nginx):"
    cat <<NGINX
  server {
      listen 443 ssl;
      server_name drop.example.com;
      ssl_certificate     /etc/ssl/certs/drop.crt;
      ssl_certificate_key /etc/ssl/private/drop.key;
      client_max_body_size 100m;
      location / {
          proxy_pass http://localhost:${port};
          proxy_set_header Host \$host;
          proxy_set_header X-Real-IP \$remote_addr;
          proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
          proxy_set_header X-Forwarded-Proto \$scheme;
      }
  }
NGINX
    echo ""
    bold "Reverse proxy (caddy):"
    echo "  drop.example.com {"
    echo "      reverse_proxy localhost:${port}"
    echo "  }"
    echo ""
    bold "Management:"
    echo "  Stop:     ${runtime} stop ${CONTAINER_NAME}"
    echo "  Start:    ${runtime} start ${CONTAINER_NAME}"
    echo "  Logs:     ${runtime} logs -f ${CONTAINER_NAME}"
    echo "  Remove:   ${runtime} rm -f ${CONTAINER_NAME}"
    echo "  Rebuild:  make setup"
    echo ""
}

main "$@"
