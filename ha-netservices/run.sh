#!/usr/bin/env bash

set -euo pipefail

source /etc/netservices/orchestrator-lib.sh

echo "Loading service registry"
load_modules
ensure_at_least_one_service_enabled

echo "Bootstrapping configuration for enabled services"
bootstrap_enabled_configs

echo "Validating enabled services"
validate_enabled_services

echo "Rendering supervisor config"
write_supervisor_config

exec /usr/bin/supervisord -n -c /tmp/supervisord.conf
