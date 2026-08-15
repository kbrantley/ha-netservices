# NetServices Home Assistant Add-on

NetServices is a modular multi-service add-on that runs:
- HAProxy
- BIND (named)
- FreeRADIUS (radiusd)
- Management API (mgmt)

The runtime is built so service lifecycle logic is shared. Each service is defined as a small module file, making it straightforward to add additional services without duplicating startup or validation code.

## Persistent Config Path

All editable runtime configuration is persisted under:

- `/config/ha-netservices`

Subfolders:
- `/config/ha-netservices/haproxy`
- `/config/ha-netservices/bind9`
- `/config/ha-netservices/radiusd`

## Service Toggles

Add-on options expose booleans:
- `enable_haproxy`
- `enable_bind`
- `enable_radiusd`
- `enable_mgmt`

At least one service must be enabled.

## Ports

- 80/tcp (HAProxy)
- 443/tcp (HAProxy)
- 53/tcp and 53/udp (BIND)
- 1812/udp and 1813/udp (RADIUS)

The Management API is intentionally not exposed as a direct published add-on port.
It is reachable through HAProxy on:

- `/mgmt/{service}/{check,reload,status}`

## Modularity Model

Shared orchestration code:
- `rootfs/etc/netservices/orchestrator-lib.sh`

Service contract and registry:
- `rootfs/etc/netservices/service-contract.sh`
- `rootfs/etc/netservices/services.registry`

Per-service modules (metadata/commands only):
- `rootfs/etc/netservices/services/*.module`

## Build

This add-on uses Home Assistant base images with `BUILD_FROM` support.
