#!/usr/bin/env bash
set -Eeuo pipefail

if [[ "${EUID}" -ne 0 ]]; then
    exec sudo -E bash "$0" "$@"
fi

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_DIR="$(cd -- "${SCRIPT_DIR}/.." && pwd)"
DATA_DIR="${NOVRO_DATA_DIR:-/data/novro}"
ENV_FILE="${DATA_DIR}/.env.docker"
TLS_DIR="${DATA_DIR}/tls"
NGINX_CONF_FILE="${DATA_DIR}/nginx.conf"
DOMAIN="${NOVRO_DOMAIN:-localhost}"
SCHEME="${NOVRO_SCHEME:-https}"
DOMAIN_EXPLICIT=false
SCHEME_EXPLICIT=false
TLS_CERT_FILE=""
TLS_KEY_FILE=""
INITIAL_PASSWORD="${NOVRO_BOOTSTRAP_PASSWORD:-novro123}"
ADMIN_EMAIL="${NOVRO_BOOTSTRAP_EMAIL:-}"
SELF_SIGNED=false
OFFLINE_IMAGES_FILE=""
OFFLINE_MODE=false

# /**
#  * usage 执行对应的运维辅助流程。
#  * @param none 无参数。
#  * @author Gao Hongshun
#  * @date 2026-08-13
#  */
usage() {
    printf '%s\n' \
        "Usage: sudo bash scripts/deploy-docker.sh [options]" \
        "" \
        "Options:" \
        "  --scheme SCHEME     http or https (default: https)" \
        "  --domain HOST       Hostname or IPv4 address (default: localhost)" \
        "  --tls-cert FILE     Trusted PEM certificate/full chain" \
        "  --tls-key FILE      PEM private key matching --tls-cert" \
        "  --offline-images FILE" \
        "                      Load an exported image archive and disable builds/pulls" \
        "  --help              Show this help"
}

while [[ "$#" -gt 0 ]]; do
    case "$1" in
        --scheme)
            [[ "$#" -ge 2 ]] || { printf '%s\n' "--scheme requires a value" >&2; exit 2; }
            SCHEME="$2"
            SCHEME_EXPLICIT=true
            shift 2
            ;;
        --domain)
            [[ "$#" -ge 2 ]] || { printf '%s\n' "--domain requires a value" >&2; exit 2; }
            DOMAIN="$2"
            DOMAIN_EXPLICIT=true
            shift 2
            ;;
        --tls-cert)
            [[ "$#" -ge 2 ]] || { printf '%s\n' "--tls-cert requires a value" >&2; exit 2; }
            TLS_CERT_FILE="$2"
            shift 2
            ;;
        --tls-key)
            [[ "$#" -ge 2 ]] || { printf '%s\n' "--tls-key requires a value" >&2; exit 2; }
            TLS_KEY_FILE="$2"
            shift 2
            ;;
        --offline-images)
            [[ "$#" -ge 2 ]] || { printf '%s\n' "--offline-images requires a value" >&2; exit 2; }
            OFFLINE_IMAGES_FILE="$2"
            OFFLINE_MODE=true
            shift 2
            ;;
        --help|-h)
            usage
            exit 0
            ;;
        *)
            printf 'Unknown option: %s\n' "$1" >&2
            usage >&2
            exit 2
            ;;
    esac
done

if [[ "${SCHEME}" != "http" && "${SCHEME}" != "https" ]]; then
    printf '%s\n' "--scheme must be http or https" >&2
    exit 2
fi
if [[ ! "${DOMAIN}" =~ ^[A-Za-z0-9.-]+$ ]]; then
    printf '%s\n' "--domain must be a hostname or IPv4 address without a scheme or port" >&2
    exit 2
fi
if [[ "${DATA_DIR}" != /* || "${DATA_DIR}" == "/" ]]; then
    printf '%s\n' "NOVRO_DATA_DIR must be an absolute path other than /" >&2
    exit 2
fi
if [[ -n "${TLS_CERT_FILE}" || -n "${TLS_KEY_FILE}" ]]; then
    if [[ "${SCHEME}" != "https" ]]; then
        printf '%s\n' "--tls-cert and --tls-key are only valid with --scheme https" >&2
        exit 2
    fi
    if [[ -z "${TLS_CERT_FILE}" || -z "${TLS_KEY_FILE}" || ! -r "${TLS_CERT_FILE}" || ! -r "${TLS_KEY_FILE}" ]]; then
        printf '%s\n' "--tls-cert and --tls-key must be provided together and be readable" >&2
        exit 2
    fi
fi
if [[ "${OFFLINE_MODE}" == true && ! -r "${OFFLINE_IMAGES_FILE}" ]]; then
    printf 'Offline image archive is not readable: %s\n' "${OFFLINE_IMAGES_FILE}" >&2
    exit 2
fi
if [[ ! -f "${ENV_FILE}" ]]; then
    if [[ "${INITIAL_PASSWORD}" =~ [[:space:]] || "${INITIAL_PASSWORD}" =~ [\$\`\\] || "${INITIAL_PASSWORD}" =~ $'\n' ]]; then
        printf '%s\n' "NOVRO_BOOTSTRAP_PASSWORD must not contain whitespace, dollar signs, backticks, or backslashes" >&2
        exit 2
    fi
    if [[ -z "${INITIAL_PASSWORD}" ]]; then
        printf '%s\n' "NOVRO_BOOTSTRAP_PASSWORD must be set for first administrator initialization" >&2
        exit 2
    fi
    if [[ "${#INITIAL_PASSWORD}" -lt 8 || ! "${INITIAL_PASSWORD}" =~ [A-Za-z] || ! "${INITIAL_PASSWORD}" =~ [0-9] ]]; then
        printf '%s\n' "NOVRO_BOOTSTRAP_PASSWORD must contain at least 8 characters, an English letter, and a digit" >&2
        exit 2
    fi
fi

# /**
#  * install_docker 执行对应的运维辅助流程。
#  * @param none 无参数。
#  * @author Gao Hongshun
#  * @date 2026-08-13
#  */
install_docker() {
    if [[ "${OFFLINE_MODE}" == true ]]; then
        local required_command
        for required_command in docker curl openssl; do
            if ! command -v "${required_command}" >/dev/null 2>&1; then
                printf '%s is required for offline deployment and must be installed before running this script.\n' "${required_command}" >&2
                exit 1
            fi
        done
        if ! docker compose version >/dev/null 2>&1; then
            printf '%s\n' "Docker Compose v2 is required for offline deployment." >&2
            exit 1
        fi
        if command -v systemctl >/dev/null 2>&1; then
            systemctl enable --now docker
        elif ! docker info >/dev/null 2>&1; then
            service docker start
        fi
        docker info >/dev/null
        docker compose version
        return 0
    fi

    if [[ ! -r /etc/os-release ]]; then
        printf '%s\n' "Cannot detect the Linux distribution" >&2
        exit 1
    fi
    # shellcheck disable=SC1091
    source /etc/os-release
    case "${ID}" in
        ubuntu|debian) ;;
        *)
            printf 'This installer supports Ubuntu and Debian hosts; detected %s. Install Docker Engine and Compose first, then rerun.\n' "${ID}" >&2
            exit 1
            ;;
    esac

    export DEBIAN_FRONTEND=noninteractive
    apt-get update
    apt-get install -y ca-certificates curl gnupg openssl

    if ! command -v docker >/dev/null 2>&1 || ! docker compose version >/dev/null 2>&1; then
        install -m 0755 -d /etc/apt/keyrings
        local repository_root="https://download.docker.com/linux/${ID}"
        curl -fsSL "${repository_root}/gpg" -o /etc/apt/keyrings/docker.asc
        chmod a+r /etc/apt/keyrings/docker.asc
        local codename="${VERSION_CODENAME:-}"
        if [[ "${ID}" == "ubuntu" && -n "${UBUNTU_CODENAME:-}" ]]; then
            codename="${UBUNTU_CODENAME}"
        fi
        [[ -n "${codename}" ]] || { printf '%s\n' "The distribution codename is unavailable" >&2; exit 1; }
        printf '%s\n' \
            "Types: deb" \
            "URIs: ${repository_root}" \
            "Suites: ${codename}" \
            "Components: stable" \
            "Architectures: $(dpkg --print-architecture)" \
            "Signed-By: /etc/apt/keyrings/docker.asc" \
            > /etc/apt/sources.list.d/docker.sources
        apt-get update
        apt-get install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
    fi

    if command -v systemctl >/dev/null 2>&1; then
        systemctl enable --now docker
    else
        service docker start
    fi
    docker info >/dev/null
    docker compose version
}

# /**
#  * read_env_value 执行对应的运维辅助流程。
#  * @param none 无参数。
#  * @author Gao Hongshun
#  * @date 2026-08-13
#  */
read_env_value() {
    local key="$1"
    if [[ ! -f "${ENV_FILE}" ]]; then
        return 0
    fi
    sed -n "s/^${key}=//p" "${ENV_FILE}" | tail -n 1
}

# /**
#  * write_env_file 执行对应的运维辅助流程。
#  * @param none 无参数。
#  * @author Gao Hongshun
#  * @date 2026-08-13
#  */
write_env_file() {
    if [[ -f "${ENV_FILE}" ]]; then
        return 0
    fi
    local database_password="$(openssl rand -hex 24)"
    local root_password="$(openssl rand -hex 24)"
    local session_secret="$(openssl rand -hex 32)"
    local provider_secret="$(openssl rand -hex 32)"
    if [[ -z "${ADMIN_EMAIL}" ]]; then
        ADMIN_EMAIL="novro@example.invalid"
    fi
    local public_url="${SCHEME}://${DOMAIN}"
    local environment="production"
    local cookie_secure="true"
    if [[ "${SCHEME}" == "http" ]]; then
        environment="development"
        cookie_secure="false"
    fi
    mkdir -p "${DATA_DIR}/mysql" "${DATA_DIR}/tls"
    umask 077
    printf '%s\n' \
        "MYSQL_DATABASE=novro" \
        "MYSQL_USER=novro_app" \
        "MYSQL_PASSWORD=${database_password}" \
        "MYSQL_ROOT_PASSWORD=${root_password}" \
        "MYSQL_IMAGE=mysql:8.4" \
        "NOVRO_IMAGE=novro:offline" \
        "NOVRO_DATA_DIR=${DATA_DIR}" \
        "NOVRO_NGINX_CONF=${NGINX_CONF_FILE}" \
        "NOVRO_PUBLIC_URL=${public_url}" \
        "NOVRO_ALLOWED_ORIGINS=${public_url}" \
        "NOVRO_ENVIRONMENT=${environment}" \
        "NOVRO_SESSION_SECRET=${session_secret}" \
        "NOVRO_PROVIDER_ENCRYPTION_SECRET=${provider_secret}" \
        "NOVRO_SESSION_TTL=24h" \
        "NOVRO_SESSION_COOKIE_NAME=novro_session" \
        "NOVRO_SESSION_COOKIE_SECURE=${cookie_secure}" \
        "NOVRO_REGISTRATION_ENABLED=true" \
        "NOVRO_REFERRAL_REWARD_BPS=1000" \
        "NOVRO_BOOTSTRAP_USERNAME=novro" \
        "NOVRO_BOOTSTRAP_EMAIL=${ADMIN_EMAIL}" \
        "NOVRO_BOOTSTRAP_DISPLAY_NAME=Novro" \
        "NOVRO_BOOTSTRAP_PASSWORD=${INITIAL_PASSWORD}" \
        "NOVRO_EMAIL_SMTP_HOST=" \
        "NOVRO_EMAIL_SMTP_PORT=587" \
        "NOVRO_EMAIL_SMTP_USERNAME=" \
        "NOVRO_EMAIL_SMTP_PASSWORD=" \
        "NOVRO_EMAIL_SMTP_TLS=true" \
        "NOVRO_EMAIL_FROM=" \
        "NOVRO_OIDC_ISSUER=" \
        "NOVRO_OIDC_CLIENT_ID=" \
        "NOVRO_OIDC_CLIENT_SECRET=" \
        "NOVRO_OIDC_DISPLAY_NAME=Enterprise account" \
        "NOVRO_OIDC_AUTO_REGISTER=true" \
        "NOVRO_EPAY_API_URL=" \
        "NOVRO_EPAY_MERCHANT_ID=" \
        "NOVRO_EPAY_MERCHANT_KEY=" \
        "NOVRO_EPAY_SITE_NAME=Novro" \
        "NOVRO_EPAY_CHANNELS=alipay,wxpay" \
        "NOVRO_HTTP_PORT=${NOVRO_HTTP_PORT:-80}" \
        "NOVRO_HTTPS_PORT=${NOVRO_HTTPS_PORT:-443}" \
        "NOVRO_BIND_ADDRESS=${NOVRO_BIND_ADDRESS:-0.0.0.0}" \
        "TZ=Asia/Shanghai" \
        > "${ENV_FILE}"
    chmod 0600 "${ENV_FILE}"
}

# /**
#  * set_env_value 执行对应的运维辅助流程。
#  * @param none 无参数。
#  * @author Gao Hongshun
#  * @date 2026-08-13
#  */
set_env_value() {
    local key="$1"
    local value="$2"
    if grep -q "^${key}=" "${ENV_FILE}"; then
        sed -i "s|^${key}=.*|${key}=${value}|" "${ENV_FILE}"
    else
        printf '%s\n' "${key}=${value}" >> "${ENV_FILE}"
    fi
}

# /**
#  * sync_runtime_env 执行对应的运维辅助流程。
#  * @param none 无参数。
#  * @author Gao Hongshun
#  * @date 2026-08-13
#  */
sync_runtime_env() {
    local public_url="${SCHEME}://${DOMAIN}"
    local environment="production"
    local cookie_secure="true"
    if [[ "${SCHEME}" == "http" ]]; then
        environment="development"
        cookie_secure="false"
    fi
    set_env_value NOVRO_NGINX_CONF "${NGINX_CONF_FILE}"
    set_env_value NOVRO_PUBLIC_URL "${public_url}"
    set_env_value NOVRO_ALLOWED_ORIGINS "${public_url}"
    set_env_value NOVRO_ENVIRONMENT "${environment}"
    set_env_value NOVRO_SESSION_COOKIE_SECURE "${cookie_secure}"
    chmod 0600 "${ENV_FILE}"
}

# /**
#  * resolve_deployment_domain 执行对应的运维辅助流程。
#  * @param none 无参数。
#  * @author Gao Hongshun
#  * @date 2026-08-13
#  */
resolve_deployment_domain() {
    local configured_url="$(read_env_value NOVRO_PUBLIC_URL)"
    if [[ ! "${configured_url}" =~ ^(http|https)://[A-Za-z0-9.-]+$ ]]; then
        printf '%s\n' "NOVRO_PUBLIC_URL in .env.docker must be an HTTP or HTTPS origin without a port or path" >&2
        exit 1
    fi
    local configured_scheme="${configured_url%%://*}"
    local configured_domain="${configured_url#*://}"
    if [[ "${SCHEME_EXPLICIT}" == false ]]; then
        SCHEME="${configured_scheme}"
    fi
    if [[ "${DOMAIN_EXPLICIT}" == false ]]; then
        DOMAIN="${configured_domain}"
    fi
}

# /**
#  * prepare_tls 执行对应的运维辅助流程。
#  * @param none 无参数。
#  * @author Gao Hongshun
#  * @date 2026-08-13
#  */
prepare_tls() {
    mkdir -p "${DATA_DIR}/mysql" "${TLS_DIR}"
    if [[ "${SCHEME}" == "http" ]]; then
        return 0
    fi
    if [[ -n "${TLS_CERT_FILE}" ]]; then
        install -m 0644 "${TLS_CERT_FILE}" "${TLS_DIR}/fullchain.pem"
        install -m 0600 "${TLS_KEY_FILE}" "${TLS_DIR}/privkey.pem"
        return 0
    fi
    if [[ -r "${TLS_DIR}/fullchain.pem" && -r "${TLS_DIR}/privkey.pem" ]]; then
        if certificate_matches_domain "${TLS_DIR}/fullchain.pem" "${DOMAIN}"; then
            return 0
        fi
        printf 'Existing TLS certificate does not match %s; generating a new self-signed certificate.\n' "${DOMAIN}"
        rm -f "${TLS_DIR}/fullchain.pem" "${TLS_DIR}/privkey.pem"
    fi
    if [[ -e "${TLS_DIR}/fullchain.pem" || -e "${TLS_DIR}/privkey.pem" ]]; then
        printf '%s\n' "Both ${TLS_DIR}/fullchain.pem and ${TLS_DIR}/privkey.pem are required" >&2
        exit 1
    fi
    local san="DNS:${DOMAIN}"
    if [[ "${DOMAIN}" =~ ^([0-9]{1,3}\.){3}[0-9]{1,3}$ ]]; then
        san="IP:${DOMAIN}"
    fi
    openssl req -x509 -nodes -newkey rsa:3072 -sha256 -days 825 \
        -keyout "${TLS_DIR}/privkey.pem" \
        -out "${TLS_DIR}/fullchain.pem" \
        -subj "/CN=${DOMAIN}" \
        -addext "subjectAltName=${san}" \
        -addext "keyUsage=digitalSignature,keyEncipherment" \
        -addext "extendedKeyUsage=serverAuth" \
        >/dev/null 2>&1
    chmod 0600 "${TLS_DIR}/privkey.pem"
    chmod 0644 "${TLS_DIR}/fullchain.pem"
    SELF_SIGNED=true
}

# /**
#  * certificate_matches_domain 执行对应的运维辅助流程。
#  * @param none 无参数。
#  * @author Gao Hongshun
#  * @date 2026-08-13
#  */
certificate_matches_domain() {
    local certificate_file="$1"
    local expected_domain="$2"
    local certificate_text
    certificate_text="$(openssl x509 -in "${certificate_file}" -noout -subject -ext subjectAltName 2>/dev/null || true)"
    if [[ -z "${certificate_text}" ]]; then
        return 1
    fi
    if printf '%s\n' "${certificate_text}" | grep -Fq "DNS:${expected_domain}"; then
        return 0
    fi
    if printf '%s\n' "${certificate_text}" | grep -Fq "IP Address:${expected_domain}"; then
        return 0
    fi
    if printf '%s\n' "${certificate_text}" | grep -Fq "CN = ${expected_domain}"; then
        return 0
    fi
    return 1
}

# /**
#  * prepare_nginx_config 执行对应的运维辅助流程。
#  * @param none 无参数。
#  * @author Gao Hongshun
#  * @date 2026-08-13
#  */
prepare_nginx_config() {
    mkdir -p "${DATA_DIR}"
    local template="${REPO_DIR}/deploy/nginx.conf"
    if [[ "${SCHEME}" == "http" ]]; then
        template="${REPO_DIR}/deploy/nginx.http.conf"
    fi
    if [[ ! -r "${template}" ]]; then
        printf 'Nginx template is missing: %s\n' "${template}" >&2
        exit 1
    fi
    install -m 0644 "${template}" "${NGINX_CONF_FILE}"
}

# /**
#  * compose 执行对应的运维辅助流程。
#  * @param none 无参数。
#  * @author Gao Hongshun
#  * @date 2026-08-13
#  */
compose() {
    local compose_files=(-f "${REPO_DIR}/compose.yaml")
    if [[ "${SCHEME}" == "http" ]]; then
        compose_files+=(-f "${REPO_DIR}/compose.http.yaml")
    fi
    docker compose --project-directory "${REPO_DIR}" --env-file "${ENV_FILE}" "${compose_files[@]}" "$@"
}

# /**
#  * load_offline_images 执行对应的运维辅助流程。
#  * @param none 无参数。
#  * @author Gao Hongshun
#  * @date 2026-08-13
#  */
load_offline_images() {
    if [[ "${OFFLINE_MODE}" != true ]]; then
        return 0
    fi
    docker load --input "${OFFLINE_IMAGES_FILE}"

    local mysql_image="$(read_env_value MYSQL_IMAGE)"
    local novro_image="$(read_env_value NOVRO_IMAGE)"
    mysql_image="${mysql_image:-mysql:8.4}"
    novro_image="${novro_image:-novro:offline}"
    docker image inspect "${mysql_image}" >/dev/null
    docker image inspect "${novro_image}" >/dev/null

    local daemon_architecture
    local expected_architecture
    daemon_architecture="$(docker info --format '{{.Architecture}}')"
    case "${daemon_architecture}" in
        amd64|x86_64) expected_architecture=amd64 ;;
        arm64|aarch64) expected_architecture=arm64 ;;
        *)
            printf 'Unsupported Docker host architecture: %s\n' "${daemon_architecture}" >&2
            exit 1
            ;;
    esac

    local image
    for image in "${mysql_image}" "${novro_image}"; do
        local image_platform
        image_platform="$(docker image inspect --format '{{.Os}}/{{.Architecture}}' "${image}")"
        if [[ "${image_platform}" != linux/* ]]; then
            printf 'Offline image %s has unsupported platform %s; a Linux image is required.\n' "${image}" "${image_platform}" >&2
            exit 1
        fi
        if [[ "${image_platform#linux/}" != "${expected_architecture}" ]]; then
            printf 'Offline image %s targets %s, but this Docker host is %s.\n' "${image}" "${image_platform}" "${daemon_architecture}" >&2
            exit 1
        fi
        printf 'Loaded offline image: %s (%s)\n' "${image}" "${image_platform}"
    done
}

# /**
#  * wait_for_ready 执行对应的运维辅助流程。
#  * @param none 无参数。
#  * @author Gao Hongshun
#  * @date 2026-08-13
#  */
wait_for_ready() {
    local public_url="$(read_env_value NOVRO_PUBLIC_URL)"
    local scheme="${public_url%%://*}"
    local host="${public_url#*://}"
    local port
    local insecure_args=()
    if [[ "${scheme}" == "https" ]]; then
        port="$(read_env_value NOVRO_HTTPS_PORT)"
        port="${port:-443}"
        insecure_args=(--insecure)
    else
        port="$(read_env_value NOVRO_HTTP_PORT)"
        port="${port:-80}"
    fi
    local attempt
    for attempt in $(seq 1 60); do
        if curl --fail --silent --show-error "${insecure_args[@]}" --resolve "${host}:${port}:127.0.0.1" "${scheme}://${host}:${port}/readyz" >/dev/null 2>&1 \
            && curl --fail --silent --show-error "${insecure_args[@]}" --resolve "${host}:${port}:127.0.0.1" "${scheme}://${host}:${port}/login" >/dev/null 2>&1; then
            return 0
        fi
        sleep 2
    done
    compose ps
    compose logs --tail=100 novro mysql || true
    printf '%s\n' "Novro did not become ready" >&2
    exit 1
}

install_docker
write_env_file
resolve_deployment_domain
sync_runtime_env
prepare_tls
prepare_nginx_config
load_offline_images
compose config --quiet
if [[ "${OFFLINE_MODE}" == true ]]; then
    compose up -d --no-build --pull never
else
    compose up -d --build
fi
wait_for_ready

bootstrap_password="$(read_env_value NOVRO_BOOTSTRAP_PASSWORD)"
if [[ -n "${bootstrap_password}" ]]; then
    sed -i 's/^NOVRO_BOOTSTRAP_PASSWORD=.*/NOVRO_BOOTSTRAP_PASSWORD=/' "${ENV_FILE}"
    if [[ "${OFFLINE_MODE}" == true ]]; then
        compose up -d --force-recreate --no-deps --no-build --pull never novro
    else
        compose up -d --force-recreate --no-deps novro
    fi
    wait_for_ready
fi
unset bootstrap_password INITIAL_PASSWORD

printf '%s\n' "Novro Docker deployment is ready: $(read_env_value NOVRO_PUBLIC_URL)"
printf '%s\n' "Application service: novro (Nginx + Go API + Next.js in one container)"
printf '%s\n' "Database service: mysql (internal network only)"
printf '%s\n' "Initial administrator username: $(read_env_value NOVRO_BOOTSTRAP_USERNAME)"
if [[ "${SELF_SIGNED}" == true ]]; then
    printf '%s\n' "A self-signed certificate was generated. Use --tls-cert and --tls-key for a trusted certificate."
fi
printf '%s\n' "The bootstrap password was removed from the runtime environment after initialization; change the known first-run password immediately in the admin console."
