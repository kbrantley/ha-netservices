#!/usr/bin/env bash

set -euo pipefail

require_service_fields() {
  local module_file="$1"
  local missing=()

  local required=(
    SERVICE_ID
    SERVICE_NAME
    SERVICE_TOGGLE_KEY
    SERVICE_RUNTIME_DIR
    SERVICE_DEFAULTS_DIR
    SERVICE_START_CMD
    SERVICE_VALIDATE_CMD
    SERVICE_HEALTH_CMD
    SERVICE_PRIORITY
  )

  local field
  for field in "${required[@]}"; do
    if [[ -z "${!field:-}" ]]; then
      missing+=("$field")
    fi
  done

  if [[ ${#missing[@]} -gt 0 ]]; then
    echo "Module ${module_file} is missing required fields: ${missing[*]}" >&2
    return 1
  fi
}

sanitize_id() {
  local value="$1"
  echo "$value" | tr -cs 'a-zA-Z0-9_-' '_' | sed 's/_$//'
}
