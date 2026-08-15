#!/usr/bin/env bash

set -euo pipefail

source /etc/netservices/service-contract.sh

REGISTRY_FILE="/etc/netservices/services.registry"
OPTIONS_FILE="/data/options.json"
SUPERVISOR_CONF="/tmp/supervisord.conf"

SERVICE_IDS=()
declare -A SERVICE_NAME_BY_ID
declare -A SERVICE_TOGGLE_BY_ID
declare -A SERVICE_RUNTIME_DIR_BY_ID
declare -A SERVICE_DEFAULTS_DIR_BY_ID
declare -A SERVICE_START_CMD_BY_ID
declare -A SERVICE_VALIDATE_CMD_BY_ID
declare -A SERVICE_HEALTH_CMD_BY_ID
declare -A SERVICE_PRIORITY_BY_ID

to_bool() {
  local value="${1:-false}"
  case "${value,,}" in
    true|1|yes|on) echo "true" ;;
    *) echo "false" ;;
  esac
}

reset_module_vars() {
  unset SERVICE_ID SERVICE_NAME SERVICE_TOGGLE_KEY SERVICE_RUNTIME_DIR
  unset SERVICE_DEFAULTS_DIR SERVICE_START_CMD SERVICE_VALIDATE_CMD
  unset SERVICE_HEALTH_CMD SERVICE_PRIORITY
}

load_modules() {
  if [[ ! -f "$REGISTRY_FILE" ]]; then
    echo "Registry file not found: $REGISTRY_FILE" >&2
    return 1
  fi

  local module_file
  while IFS= read -r module_file || [[ -n "$module_file" ]]; do
    [[ -z "$module_file" || "$module_file" == \#* ]] && continue

    if [[ ! -f "$module_file" ]]; then
      echo "Module file not found: $module_file" >&2
      return 1
    fi

    reset_module_vars
    # shellcheck disable=SC1090
    source "$module_file"
    require_service_fields "$module_file"

    local safe_id
    safe_id="$(sanitize_id "$SERVICE_ID")"

    SERVICE_IDS+=("$safe_id")
    SERVICE_NAME_BY_ID["$safe_id"]="$SERVICE_NAME"
    SERVICE_TOGGLE_BY_ID["$safe_id"]="$SERVICE_TOGGLE_KEY"
    SERVICE_RUNTIME_DIR_BY_ID["$safe_id"]="$SERVICE_RUNTIME_DIR"
    SERVICE_DEFAULTS_DIR_BY_ID["$safe_id"]="$SERVICE_DEFAULTS_DIR"
    SERVICE_START_CMD_BY_ID["$safe_id"]="$SERVICE_START_CMD"
    SERVICE_VALIDATE_CMD_BY_ID["$safe_id"]="$SERVICE_VALIDATE_CMD"
    SERVICE_HEALTH_CMD_BY_ID["$safe_id"]="$SERVICE_HEALTH_CMD"
    SERVICE_PRIORITY_BY_ID["$safe_id"]="$SERVICE_PRIORITY"
  done < "$REGISTRY_FILE"

  if [[ ${#SERVICE_IDS[@]} -eq 0 ]]; then
    echo "No services found in registry." >&2
    return 1
  fi
}

option_value() {
  local key="$1"
  local default="$2"

  if [[ ! -f "$OPTIONS_FILE" ]]; then
    echo "$default"
    return 0
  fi

  local value
  value="$(jq -r --arg key "$key" 'if has($key) then .[$key] else empty end' "$OPTIONS_FILE" 2>/dev/null || true)"

  if [[ -z "$value" || "$value" == "null" ]]; then
    echo "$default"
  else
    echo "$value"
  fi
}

is_service_enabled() {
  local service_id="$1"
  local toggle_key="${SERVICE_TOGGLE_BY_ID[$service_id]}"

  local raw
  raw="$(option_value "$toggle_key" "true")"
  [[ "$(to_bool "$raw")" == "true" ]]
}

enabled_service_ids() {
  local id
  for id in "${SERVICE_IDS[@]}"; do
    if is_service_enabled "$id"; then
      echo "$id"
    fi
  done
}

copy_if_missing() {
  local src="$1"
  local dst="$2"

  if [[ -d "$src" ]]; then
    mkdir -p "$dst"
    cp -an "$src"/. "$dst"/
  elif [[ -f "$src" ]]; then
    mkdir -p "$(dirname "$dst")"
    if [[ ! -f "$dst" ]]; then
      cp -a "$src" "$dst"
    fi
  fi
}

bootstrap_service_configs() {
  local id="$1"
  local defaults_dir="${SERVICE_DEFAULTS_DIR_BY_ID[$id]}"
  local runtime_dir="${SERVICE_RUNTIME_DIR_BY_ID[$id]}"

  mkdir -p "$runtime_dir"
  copy_if_missing "$defaults_dir" "$runtime_dir"

  if [[ "$id" == "bind" ]]; then
    mkdir -p "$runtime_dir/cache"
  fi
}

bootstrap_enabled_configs() {
  local id
  while IFS= read -r id; do
    [[ -z "$id" ]] && continue
    bootstrap_service_configs "$id"
  done < <(enabled_service_ids)
}

validate_enabled_services() {
  local id cmd
  while IFS= read -r id; do
    [[ -z "$id" ]] && continue
    cmd="${SERVICE_VALIDATE_CMD_BY_ID[$id]}"
    echo "Validating $id"
    if ! bash -c "$cmd"; then
      echo "Validation failed for $id" >&2
      return 1
    fi
  done < <(enabled_service_ids)
}

healthcheck_enabled_services() {
  local id cmd
  while IFS= read -r id; do
    [[ -z "$id" ]] && continue
    cmd="${SERVICE_HEALTH_CMD_BY_ID[$id]}"
    if ! bash -c "$cmd"; then
      echo "Health check failed for $id" >&2
      return 1
    fi
  done < <(enabled_service_ids)
}

write_supervisor_config() {
  cat > "$SUPERVISOR_CONF" <<'EOF'
[supervisord]
nodaemon=true
logfile=/dev/null
pidfile=/tmp/supervisord.pid

EOF

  local id name cmd priority
  while IFS= read -r id; do
    [[ -z "$id" ]] && continue
    name="${SERVICE_NAME_BY_ID[$id]}"
    cmd="${SERVICE_START_CMD_BY_ID[$id]}"
    priority="${SERVICE_PRIORITY_BY_ID[$id]}"

    cat >> "$SUPERVISOR_CONF" <<EOF
[program:${id}]
command=${cmd}
autostart=true
autorestart=true
startretries=5
priority=${priority}
stdout_logfile=/dev/fd/1
stdout_logfile_maxbytes=0
stderr_logfile=/dev/fd/2
stderr_logfile_maxbytes=0

EOF

    echo "Enabled ${name} (${id})"
  done < <(enabled_service_ids)
}

ensure_at_least_one_service_enabled() {
  local count
  count="$(enabled_service_ids | wc -l | tr -d ' ')"
  if [[ "$count" == "0" ]]; then
    echo "No services are enabled. Enable at least one service option." >&2
    return 1
  fi
}
