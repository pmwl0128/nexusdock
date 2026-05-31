#!/usr/bin/env bash
set -u

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT_DIR"

failures=0
warns=0

ok() { printf '[OK] %s\n' "$*"; }
warn() { warns=$((warns+1)); printf '[WARN] %s\n' "$*"; }
fail() { failures=$((failures+1)); printf '[FAIL] %s\n' "$*"; }

compose_cmd() {
  if docker compose version >/dev/null 2>&1; then
    printf 'docker compose'
    return 0
  fi
  if command -v docker-compose >/dev/null 2>&1; then
    printf 'docker-compose'
    return 0
  fi
  return 1
}

trim() {
  sed 's/^[[:space:]]*//;s/[[:space:]]*$//'
}

read_env_file() {
  local file="$1"
  local line key value first last
  while IFS= read -r line || [ -n "$line" ]; do
    line="${line%$'\r'}"
    case "$line" in
      ''|'#'*) continue ;;
    esac
    case "$line" in
      *=*) ;;
      *) continue ;;
    esac
    key="$(printf '%s' "${line%%=*}" | trim)"
    value="$(printf '%s' "${line#*=}" | trim)"
    first="${value:0:1}"
    last="${value: -1}"
    if { [ "$first" = '"' ] && [ "$last" = '"' ]; } || { [ "$first" = "'" ] && [ "$last" = "'" ]; }; then
      value="${value:1:${#value}-2}"
    fi
    case "$key" in
      MEMORYDOCK_*) export "$key=$value" ;;
    esac
  done < "$file"
}

load_env() {
  if [ -f .env ]; then
    read_env_file .env
    ok '.env 已存在并已安全加载'
  elif [ -f ../.env ]; then
    read_env_file ../.env
    ok '../.env 已存在并已安全加载（部署目录）'
  else
    warn '.env 不存在；将仅使用当前环境变量和默认值检查'
  fi
}

check_compose() {
  local cmd
  if cmd="$(compose_cmd)"; then
    ok "Compose 命令可用：$cmd"
  else
    fail '未找到 docker compose 或 docker-compose'
  fi
}

check_auth_policy() {
  local host="${MEMORYDOCK_HOST:-127.0.0.1}"
  local require="${MEMORYDOCK_REQUIRE_AUTH:-false}"
  local user="${MEMORYDOCK_USERNAME:-}"
  local pass="${MEMORYDOCK_PASSWORD:-}"
  local token="${MEMORYDOCK_AUTH_TOKEN:-}"
  if [ "$require" = "true" ]; then
    if [ -z "$token" ]; then fail 'MEMORYDOCK_REQUIRE_AUTH=true 但 MEMORYDOCK_AUTH_TOKEN 为空'; else ok 'API Bearer token 已配置'; fi
    if [ -z "$user" ] && [ -z "${MEMORYDOCK_PASSWORD_HASH:-}" ]; then fail 'MEMORYDOCK_REQUIRE_AUTH=true 但 UI Basic Auth 未配置'; fi
    if [ "$user" = "admin" ] && [ "$pass" = "memorydock" ]; then fail '禁止公网/强认证模式使用默认账号密码 admin/memorydock'; fi
  elif [ "$host" != "127.0.0.1" ] && [ "$host" != "localhost" ]; then
    warn "MEMORYDOCK_HOST=$host 不是 localhost，建议设置 MEMORYDOCK_REQUIRE_AUTH=true"
  else
    ok '认证策略检查通过'
  fi
}

check_memory_repo() {
  local dir="${MEMORYDOCK_STORE_DIR:-memory}"
  if [ ! -d "$dir" ] && [ "$dir" = "memory" ] && [ -d ../memory ]; then
    dir="../memory"
  fi
  if [ -d "$dir" ]; then ok "记忆目录存在：$dir"; else warn "记忆目录不存在：$dir"; fi
  if [ -d "$dir/.git" ]; then
    ok "记忆目录是 Git 仓库：$dir"
    if git -C "$dir" status --short --branch >/tmp/memorydock-doctor-git-status 2>&1; then
      sed 's/^/[GIT] /' /tmp/memorydock-doctor-git-status
    else
      warn '记忆 Git 状态读取失败'
    fi
    if git -C "$dir" remote -v | grep -q .; then ok 'Git remote 已配置'; else warn '记忆 Git remote 未配置'; fi
    if git -C "$dir" fetch --dry-run >/tmp/memorydock-doctor-fetch 2>&1; then ok 'Git remote 可 fetch'; else warn "Git remote fetch 失败：$(tr '\n' ' ' </tmp/memorydock-doctor-fetch | cut -c1-240)"; fi
  else
    warn "记忆目录不是 Git 仓库：$dir"
  fi
}

check_ports_and_health() {
  local port="${MEMORYDOCK_PORT:-18777}"
  if command -v lsof >/dev/null 2>&1; then
    if lsof -nP -iTCP:"$port" -sTCP:LISTEN >/dev/null 2>&1; then ok "端口 $port 正在监听"; else warn "端口 $port 未监听"; fi
  else
    warn 'lsof 不可用，跳过端口监听检查'
  fi
  if curl -fsS "http://127.0.0.1:${port}/health" >/tmp/memorydock-doctor-health 2>&1; then
    ok "本地 health 通过：http://127.0.0.1:${port}/health"
  else
    warn "本地 health 失败：http://127.0.0.1:${port}/health"
  fi
  if [ -n "${MEMORYDOCK_PUBLIC_HEALTH_URL:-}" ]; then
    if curl -fsS "$MEMORYDOCK_PUBLIC_HEALTH_URL" >/tmp/memorydock-doctor-public-health 2>&1; then ok "公网 health 通过：$MEMORYDOCK_PUBLIC_HEALTH_URL"; else warn "公网 health 失败：$MEMORYDOCK_PUBLIC_HEALTH_URL"; fi
  fi
}

check_assets() {
  if [ -f internal/httpx/web_dist/index.html ]; then ok '嵌入式前端 web_dist 存在'; else fail 'internal/httpx/web_dist/index.html 不存在，请先 npm run build'; fi
  if git ls-files --error-unmatch web/tsconfig.tsbuildinfo >/dev/null 2>&1; then fail 'web/tsconfig.tsbuildinfo 仍被 Git 追踪'; else ok 'web/tsconfig.tsbuildinfo 未被 Git 追踪'; fi
}

load_env
check_compose
check_auth_policy
check_memory_repo
check_ports_and_health
check_assets

printf '\nDoctor finished: %d failure(s), %d warning(s).\n' "$failures" "$warns"
[ "$failures" -eq 0 ]
