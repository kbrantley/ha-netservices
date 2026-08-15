#!/usr/bin/env bash

set -euo pipefail
trap 'echo "NetServices startup failed at line ${LINENO}" >&2' ERR

source /etc/netservices/orchestrator-lib.sh

echo "NetServices startup beginning"
echo "Loading service registry"
load_modules
ensure_at_least_one_service_enabled

echo "Bootstrapping configuration for enabled services"
bootstrap_enabled_configs

echo "Validating enabled services"
if ! validate_enabled_services; then
	echo "Preflight validation reported errors; continuing so service logs can surface the failure" >&2
fi

echo "Rendering supervisor config"
write_supervisor_config

exec /usr/bin/supervisord -n -c /tmp/supervisord.conf
