# Update CPA Manager Plus And CPA

CPAMP now has two update paths: Integrated Full deployments use managed UI updates, while separated, External, and Slim deployments without integrated CPA keep using image, installer, or external CPA update workflows.

## UI Hierarchy

### Dashboard: Lightweight Detection

The Dashboard version card continues to check CPAMP and CPA versions automatically. Integrated/Full uses the signed runtime manifest, while external and split deployments keep the public version endpoints. Its job is to show:

- Current and discovered versions.
- Whether an update appears available.
- A “Go to System updates” link for Integrated sessions instead of changing binaries directly.

The actual update belongs on System so release notes, compatibility requirements, impact, progress, and rollback are visible before the user confirms a change.

### System → Runtime & Updates: Complete Flow

This section shows:

- Deployment mode and managed capabilities.
- Current and available CPAMP/CPA versions.
- Release pages, release notes, and the minimum CPAMP version required by CPA.
- “Update CPAMP,” “Update CPA,” and “Update all.”
- Confirmation, progress, component results, failure details, and rollback results.

When an update starts, the browser receives a one-time operation token. If CPAMP Manager is briefly unavailable while its binary changes, the page can still query that one operation through the Gateway runtime endpoint. The token cannot perform other management actions.

## Managed Update Support

| Mode                                                   | Managed CPAMP update | Managed CPA update                 |
| ------------------------------------------------------ | -------------------- | ---------------------------------- |
| Integrated Full Docker / Native                        | Yes                  | Yes                                |
| Slim after downloading CPA and switching to Integrated | Yes                  | Yes                                |
| Slim using an existing CPA                             | No                   | No; update external CPA separately |
| Separated `installer-managed` CPA + CPAMP              | No                   | No; use Compose or the installer   |
| External / CPA Panel                                   | No                   | No                                 |

Managed updates verify the signed release manifest, platform/architecture/libc, asset size, and SHA-256. Each restarted component must pass health checks. If a later component fails, previously switched components are rolled back in reverse order.

The manifest attached to the latest stable CPAMP release is refreshed every six hours and can also be refreshed manually. This lets a new standalone CPA release appear in the UI without waiting for another CPAMP release. If the repository-level `CPA_RUNTIME_VERSION` pin is configured, both release packaging and scheduled manifest refresh deliberately stay on that CPA version until the pin changes.

The signed manifest declares the minimum CPAMP version required by CPA. When the current CPAMP is too old, the UI disables “Update CPA.” Use “Update all” when the CPAMP version in the manifest satisfies the requirement. If the selected release channel still has no compatible CPAMP, “Update all” is disabled as well. Scheduled refreshes conservatively use the current stable CPAMP as the compatibility floor for a newly published CPA, so an older CPAMP may need to update with it.

Shutdown budgets increase by layer: CPA may need about 30 seconds, the supervisor allows 35 seconds, the Integrated runtime allows 40 seconds, Docker CPAMP and installer stop operations allow 45 seconds, and the Windows native control script allows 50 seconds. Do not configure a shorter grace period in custom Compose, systemd, or another process manager; a normal graceful shutdown could otherwise be mistaken for failure and be force-killed or rolled back.

## Before Updating

1. Read the target Release Notes and compatibility requirements.
2. Back up the complete data directory:
   - `usage.sqlite`, `usage.sqlite-wal`, and `usage.sqlite-shm`.
   - `data.key`.
   - `cpa/config.yaml`, `cpa/auths/`, and `cpa/logs/`.
   - `runtime/`, which holds installed components and rollback state.
3. Confirm that no second Manager Server uses the same SQLite or consumes the same CPA usage queue.
4. For public deployments, confirm that the reverse proxy allows the current dynamic Base Path.

## Integrated UI Update

1. Open “System → Runtime & Updates.”
2. Click “Check for updates.”
3. Review both component versions, release notes, and compatibility requirements.
4. Select the scope and confirm.
5. Keep the page open until the operation reaches `succeeded`, `failed`, or `rolled_back`.

Status meanings:

| Status               | Meaning                                                                                   |
| -------------------- | ----------------------------------------------------------------------------------------- |
| `queued` / `running` | Verifying, installing, or restarting components                                           |
| `handoff_pending`    | The new CPAMP is installed; the runtime is restarting and awaiting readiness confirmation |
| `rolling_back`       | The update failed and applied components are being restored                               |
| `succeeded`          | Selected components passed health checks and the replacement runtime confirmed readiness  |
| `rolled_back`        | The update failed, but the previous version was restored                                  |
| `failed`             | Update or rollback did not fully succeed; inspect component errors and recover manually   |

Integrated Docker stores updated binaries under `/data/runtime/components/`. On container restart, the image launcher reads runtime state and delegates to the active version. Periodically refresh the base image as well for operating-system and image-layer fixes.

## Separated Docker From The Installer

Enter the original install directory:

```bash
cd "$HOME/cpa-manager-plus"
docker compose pull
docker compose up -d
docker compose ps
```

Update only CPAMP:

```bash
docker compose pull cpa-manager-plus
docker compose up -d cpa-manager-plus
```

Update only CPA:

```bash
docker compose pull cli-proxy-api
docker compose up -d cli-proxy-api
```

Installer upgrades do not regenerate the CPAMP Admin Key or overwrite CPA `config.yaml`, auths, logs, Docker data volumes, or custom Compose files. Before recreating services, the installer records the current CPAMP image and, for a separated stack, the CPA image. If an old image ID cannot be read, the image no longer exists, or Compose uses an immutable digest reference that cannot be retagged, the installer fails before `pull` so it does not enter an upgrade that cannot be rolled back automatically. If the new deployment never becomes healthy, it retags the saved image IDs, recreates the previous services without pulling, and verifies health again. Use an explicit installer operation for repair or configuration regeneration; do not treat `CPAMP_OVERWRITE=1` as a normal update shortcut.

## Manual Docker

Keep the original `/data` volume attached:

```bash
docker pull seakee/cpa-manager-plus:latest
docker stop cpa-manager-plus
docker rm cpa-manager-plus
docker run -d \
  --name cpa-manager-plus \
  --restart unless-stopped \
  -p 18137:18137 \
  -v cpa-manager-plus-data:/data \
  seakee/cpa-manager-plus:latest
```

Continue mapping `8137` or `18317` while older clients use them. All three ports have the same capabilities.

## Native Packages

### Older Native Install Managed By The Installer

Download the new installer and choose upgrade, or run non-interactively:

```bash
CPAMP_OPERATION=upgrade \
CPAMP_NON_INTERACTIVE=1 \
CPAMP_CONFIRM=1 \
bash install-cpamp.sh
```

The installer detects the old `run.sh`, PID/service, and SQLite. It preserves data, secrets, CPA state, and the old runtime. The archive is extracted into a staging directory before the target version directory changes, so an interrupted attempt can be retried with the same version. It stops the old process only after the new package and startup files are ready. If the new version does not become healthy, it restores the old run script and attempts to restart the previous version.

systemd and other external process managers are outside the installer PID lifecycle. Stop the external service first, run the upgrade, then reinstall or reload the service file generated in the install directory.

### Manual Replacement

1. Stop the process.
2. Back up the complete data directory and old package.
3. Download the matching `_full` or `_slim` package.
4. Keep using the existing `CPA_MANAGER_RUNTIME_DATA_DIR` / `USAGE_DATA_DIR`.
5. Start the new package and verify `18137/health`.

Do not replace an existing data directory with an empty one, and do not copy only `usage.sqlite` while omitting WAL/SHM and `data.key`.

## Verification And Recovery

```bash
curl http://127.0.0.1:18137/health
curl http://127.0.0.1:18137/usage-service/info
curl -H "Authorization: Bearer <CPAMP_ADMIN_KEY>" \
  http://127.0.0.1:18137/status
```

Inspect:

```text
deployment.mode
runtimeAvailable
components.cpamp
components.cpa
latestOperationId
operations
collector.lastError
```

Common failures return a specific error code and documentation link in the UI. Continue with [Setup Troubleshooting](../troubleshooting/setup.md), [Backup And Restore](./backup.md), and [Integrated Runtime Migration](../migration/integrated-runtime.md).
