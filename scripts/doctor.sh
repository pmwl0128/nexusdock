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
      NEXUS_*|RECALL_*) export "$key=$value" ;;
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
  local host="${NEXUS_HOST:-127.0.0.1}"
  local require="${NEXUS_REQUIRE_AUTH:-false}"
  local token="${NEXUS_AUTH_TOKEN:-}"
  if [ "$require" = "true" ]; then
    if [ -z "$token" ]; then fail 'NEXUS_REQUIRE_AUTH=true 但 NEXUS_AUTH_TOKEN 为空'; else ok '程序化 API Bearer token 已配置'; fi
  elif [ "$host" != "127.0.0.1" ] && [ "$host" != "localhost" ]; then
    warn "NEXUS_HOST=$host 不是 localhost，建议为程序化 API 设置 NEXUS_REQUIRE_AUTH=true"
  else
    ok '程序化 API 认证策略检查通过'
  fi

  local legacy_key
  for legacy_key in NEXUS_USERNAME NEXUS_PASSWORD NEXUS_PASSWORD_HASH NEXUS_ACCESS_FILE; do
    if [ -n "${!legacy_key:-}" ]; then
      fail "$legacy_key 已退出；管理员账号由 Nexus 数据库和本地 admin 命令管理"
    fi
  done
}

check_nexus_data_dir() {
  local dir="${NEXUS_DATA_DIR:-./nexus-data}"
  if mkdir -p "$dir" 2>/tmp/nexus-doctor-mkdir; then ok "Nexus 数据目录存在且可创建：$dir"; else fail "Nexus 数据目录不可创建：$dir $(cat /tmp/nexus-doctor-mkdir 2>/dev/null)"; return; fi
  if [ -w "$dir" ]; then ok "Nexus 数据目录可写：$dir"; else fail "Nexus 数据目录不可写：$dir"; fi
  local db="$dir/nexus.db"
  if [ -f "$db" ]; then
    if command -v sqlite3 >/dev/null 2>&1; then
      if sqlite3 "$db" 'PRAGMA quick_check;' | grep -qx ok; then ok "nexus.db quick_check 通过：$db"; else fail "nexus.db quick_check 失败：$db"; fi
    else
      warn 'sqlite3 不可用，跳过 nexus.db quick_check'
    fi
  else
    warn "nexus.db 不存在，可能是首次启动：$db"
  fi
}

check_recall_repo() {
  local dir="${RECALL_REPO_DIR:-recall}"
  if [ -d "$dir" ]; then ok "Recall 仓库目录存在：$dir"; else warn "Recall 仓库目录不存在：$dir"; fi
  if [ -d "$dir/.git" ]; then
    ok "Recall 仓库是 Git 仓库：$dir"
    if git -C "$dir" status --short --branch >/tmp/nexus-doctor-git-status 2>&1; then
      sed 's/^/[GIT] /' /tmp/nexus-doctor-git-status
    else
      warn '记忆 Git 状态读取失败'
    fi
    if git -C "$dir" remote -v | grep -q .; then ok 'Git remote 已配置'; else warn '记忆 Git remote 未配置'; fi
    if git -C "$dir" fetch --dry-run >/tmp/nexus-doctor-fetch 2>&1; then ok 'Git remote 可 fetch'; else warn "Git remote fetch 失败：$(tr '\n' ' ' </tmp/nexus-doctor-fetch | cut -c1-240)"; fi
  else
    warn "Recall 仓库不是 Git 仓库：$dir"
  fi
}

check_ports_and_health() {
  local port="${NEXUS_PORT:-18777}"
  if command -v lsof >/dev/null 2>&1; then
    if lsof -nP -iTCP:"$port" -sTCP:LISTEN >/dev/null 2>&1; then ok "端口 $port 正在监听"; else warn "端口 $port 未监听"; fi
  else
    warn 'lsof 不可用，跳过端口监听检查'
  fi
  if curl -fsS "http://127.0.0.1:${port}/health" >/tmp/nexus-doctor-health 2>&1; then
    ok "本地 health 通过：http://127.0.0.1:${port}/health"
  else
    warn "本地 health 失败：http://127.0.0.1:${port}/health"
  fi
  if curl -fsS "http://127.0.0.1:${port}/v1/auth/status" >/tmp/nexus-doctor-auth-status 2>&1; then
    if grep -Eq '"initialized"[[:space:]]*:[[:space:]]*true' /tmp/nexus-doctor-auth-status; then
      ok '管理员账号已初始化'
    else
      warn '管理员账号尚未初始化；请在本机运行 nexusdock admin init'
    fi
  else
    warn '管理员状态接口不可用'
  fi
  local public_url="${NEXUS_PUBLIC_HEALTH_URL:-}"
  if [ -n "$public_url" ]; then
    if curl -fsS "$public_url" >/tmp/nexus-doctor-public-health 2>&1; then ok "公网 health 通过：$public_url"; else warn "公网 health 失败：$public_url"; fi
  fi
}

check_assets() {
  if [ -f internal/httpx/web_dist/index.html ]; then ok '嵌入式前端 web_dist 存在'; else fail 'internal/httpx/web_dist/index.html 不存在，请先 npm run build'; fi
  if git ls-files --error-unmatch web/tsconfig.tsbuildinfo >/dev/null 2>&1; then fail 'web/tsconfig.tsbuildinfo 仍被 Git 追踪'; else ok 'web/tsconfig.tsbuildinfo 未被 Git 追踪'; fi
}

load_env
check_compose
check_auth_policy
check_nexus_data_dir
check_recall_repo
check_ports_and_health
check_assets

printf '\nDoctor finished: %d failure(s), %d warning(s).\n' "$failures" "$warns"
[ "$failures" -eq 0 ]
