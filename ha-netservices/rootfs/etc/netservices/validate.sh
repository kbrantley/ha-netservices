#!/usr/bin/env bash

set -euo pipefail

source /etc/netservices/orchestrator-lib.sh

load_modules
ensure_at_least_one_service_enabled
bootstrap_enabled_configs
validate_enabled_services

echo "All enabled services validated successfully."
