---
title: Migrate To The Bundled CPA Runtime
description: Move an older CPAMP Docker or native deployment to Full/Slim, the three-port gateway, UI setup, dynamic Base Path, and managed updates without losing data.
---

# Migrate To The Bundled CPA Runtime

This page is for users already running CPA Manager Plus before the bundled runtime release. You do not have to rebuild first: the old `18317` mapping, SQLite, `data.key`, admin credential, and CPA connection remain usable. Add `18137`, `8137`, bundled CPA, and a dynamic Base Path gradually.

## How An Older Deployment Is Inferred

When `CPA_MANAGER_DEPLOYMENT_MODE` is not explicit, runtime checks in this order:

1. `CPA_UPSTREAM_URL` exists: start as installer-managed and do not launch bundled CPA.
2. SQLite `manager_config_v1` or legacy `setup` contains a CPA URL: keep it as external and do not launch bundled CPA.
3. No old CPA connection and a CPA binary is present: start Integrated/Full.
4. No CPA binary: start Slim.

The legacy SQLite check is read-only. Manager Server then opens the same database and performs its normal compatible schema migrations.

## Data That Must Stay Together

Stop the old process before migration or rollback and back up:

```text
usage.sqlite
usage.sqlite-wal
usage.sqlite-shm
data.key
```

A split CPA deployment must also preserve:

```text
cliproxyapi/config.yaml
cliproxyapi/auths/
cliproxyapi/logs/
secrets/cpa-management-key
```

Never run old and new Manager Servers against the same CPA queue. The new runtime reserves all gateway and internal control listeners before it starts Manager or CPA children, so a port conflict cannot start a second collector first.

## Smooth Docker Upgrade

1. Back up the named volume or host data directory.
2. Run `docker compose pull && docker compose up -d` in the existing Compose directory.
3. First verify login, history, and the CPA connection through `http://<host>:18317/management.html`.
4. Then add all mappings to the same container:

```yaml
ports:
  - '8137:8137'
  - '18137:18137'
  - '18317:18317'
```

All three ports use the same gateway Handler and expose the same API/proxy behavior. New documentation uses `18137` as the management entry; existing clients can keep `18317` while they migrate.

For an in-place upgrade through the new installer, the script confirms before pulling that the previous images can be used for rollback. It stops when an image ID is missing, the local image is unavailable, or Compose uses a digest reference. Newly generated Compose files allow 45 seconds for CPAMP and 35 seconds for separated CPA to stop. Older Compose files remain unchanged; the installer explicitly uses a 45-second stop window.

An older split Compose deployment can keep its CPA container and `CPA_UPSTREAM_URL`. Do not delete old CPA data merely because the CPAMP image now contains CPA. Move to Integrated only after validating bundled CPA config, auths, logs, and Management Key.

## Native Package Migration

- An older native deployment created by the one-click installer can run the new script and choose “upgrade existing deployment,” or set `CPAMP_OPERATION=upgrade`. The installer downloads and writes the new startup files before stopping the old process. It preserves SQLite/WAL/SHM, `data.key`, CPA config/auths/logs, and the old runtime; a failed health check restores the previous run script and attempts to restart the old version.
- To keep an existing CPA, download `_slim` or the unsuffixed compatibility asset, keep the existing `USAGE_DATA_DIR` / `USAGE_DB_PATH`, and let setup or legacy SQLite provide the CPA connection.
- To move to bundled CPA, download `_full`, start in `runtime` mode, and validate the bundled CPA data directory during a maintenance window before stopping the old CPA.
- `.cpamp-runtime-mode` and the bundled control script select `slim` or `integrated`. Explicit `serve` still overrides the default, but does not provide the gateway or managed updates.

If systemd or another external process manager owns the old process, stop that service first, run the installer upgrade, and reload the generated service file. The installer does not terminate an unknown process when PID ownership cannot be confirmed.

## Admin Key And Setup

An existing CPAMP admin credential remains valid and does not force setup again. Only fresh installs stop generating an Admin Key in the installer and use a one-time bootstrap token to set it in the UI.

If an old CPA connection exists but no CPAMP admin credential does, setup skips the CPA step and asks only for an Admin Key. Slim without a CPA connection can download the latest CPA or use an existing one.

## Base Path And Reverse Proxy

The default remains `/management.html`. After changing it to `/admin`, `/panel`, or `/` in System, the new entry is immediate and the retired entry returns 404. Update reverse-proxy health checks and routes before switching a public entry. Base Path is not an authentication control.

## Update Capability

Only Integrated/Full deployments can manage both CPAMP and CPA updates in System. Installer-managed and external deployments keep their previous upgrade workflow. Slim becomes Integrated after it successfully downloads CPA.

## Rollback

Stop the new runtime before restoring an older binary or image and mount the same backed-up data. If the new release wrote a schema migration, prefer the complete pre-upgrade SQLite + WAL/SHM + `data.key` backup. Never run both versions at once.
