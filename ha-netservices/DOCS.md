# NetServices Add-on Documentation

## Runtime Flow

1. `run.sh` loads the service registry.
2. Enabled services are discovered from `/data/options.json`.
3. Missing runtime config files are initialized from `/etc/defaults/<service>`.
4. Each enabled service is validated through the shared validation loop.
5. A Supervisor config is generated from service metadata.
6. Supervisor starts and keeps enabled services running.

## Management API

The add-on includes a small Go management service built during Docker image build.
It is routed through HAProxy at:

- `/mgmt/{service}/{check,reload,status}`

Current services:
- `haproxy`
- `bind`
- `radiusd`

Current action semantics:
- `check`: runs the service syntax/validation command.
- `status`: returns a process identifier when running.
- `reload`:
	- HAProxy: validates config, then sends `SIGUSR2` to the HAProxy master process.
	- BIND: sends `SIGHUP` to `named`.
	- FreeRADIUS: sends `SIGTERM` and waits for supervisor to restart `radiusd`.

The API returns JSON including success/failure, exit code, stdout/stderr, and duration.

## Default Files

- HAProxy defaults: `rootfs/etc/defaults/haproxy/haproxy.cfg`
- BIND defaults: `rootfs/etc/defaults/bind9/named.conf`
- FreeRADIUS defaults: copied from `/etc/raddb` into `/etc/defaults/radiusd` during image build

## VS Code Editing

Because all runtime configs live in `/config/ha-netservices`, they are visible in Home Assistant's config filesystem and can be edited from the VS Code add-on.

Recommended workflow:
1. Edit files under `/config/ha-netservices/<service>`.
2. Restart the NetServices add-on.
3. Check logs for validation/startup output.

## Adding Another Service

To add service `foo`:

1. Add package install in `Dockerfile`.
2. Create `rootfs/etc/netservices/services/foo.module` with required contract fields.
3. Add module path to `rootfs/etc/netservices/services.registry`.
4. Add defaults in `rootfs/etc/defaults/foo`.
5. Add `enable_foo` option and schema bool in `config.yaml`.

No new orchestration code should be required.

## Constraints

- Keep service modules declarative.
- Keep startup/validation/health logic centralized in `orchestrator-lib.sh`.
- Do not duplicate service lifecycle control flow in module files.
