# One-Click Installer

Use the installer for a first deployment, or when CPA is already running and you only want to bring up CPAMP. It does not overwrite existing config files by default. Before it writes files or starts services, it shows a summary and asks for confirmation.

Most users only need four steps: run the script, choose the install scope, choose Docker or a native package, and confirm the summary. Then open the `18137` management entry and set the CPAMP Admin Key in the UI.

## Run It

Download the script, then run it:

```bash
curl -fsSLO https://raw.githubusercontent.com/seakee/CPA-Manager-Plus/main/bin/install-cpamp.sh
bash install-cpamp.sh
```

If you want to inspect it first:

```bash
less install-cpamp.sh
bash install-cpamp.sh
```

The wizard walks through:

1. Detecting OS, architecture, WSL, ports, and required commands.
2. Choosing the operation language.
3. Detecting a local CPA and validating its Management Key before it can be selected.
4. Choosing CPA + CPAMP Full or CPAMP Slim, then Docker or a native package.
5. Generating minimal config files and only the CPA secrets required by the selected mode.
6. Showing a summary so you can confirm, modify, or abort.
7. Running the install only after confirmation.

## Supported Combinations

| Install scope |    Docker | Native package |
| ------------- | --------: | -------------: |
| CPA + CPAMP   | Supported |      Supported |
| CPAMP only    | Supported |      Supported |

Native releases provide both `_full` and `_slim` packages: `_full` bundles CPA, while `_slim` contains CPAMP only. Docker publishes one image that includes CPA; when the installer selects CPAMP only, that same image runs in Slim mode without starting its bundled CPA. Both Slim paths can download the latest CPA during setup or keep an existing CPA.

## Full Docker Install

Choose this when CPA is not installed yet. Docker uses separate CPA and CPAMP containers; native mode downloads the `_full` package with bundled CPA. Both leave CPAMP Admin Key creation to the setup UI.

::: details Generated files and connection behavior

When you choose CPA + CPAMP, the script generates:

```text
compose.yaml
.env
secrets/cpa-management-key
secrets/cpa-demo-client-key
cliproxyapi/config.yaml
cliproxyapi/auths/
cliproxyapi/logs/
```

Generated keys use these formats by default:

```text
CPA Management Key: cpa_ + 32 alphanumeric characters
Demo client API key: sk- + 64 alphanumeric characters
```

When rerun, the installer reuses existing non-empty single-line secret files as-is, so manually managed keys do not have to match the default generated format.

The CPA minimal config enables remote management and usage publishing:

```yaml
api-keys:
  - 'sk-...'

remote-management:
  secret-key: 'cpa_...'
  allow-remote: true

usage-statistics-enabled: true
redis-usage-queue-retention-seconds: 60
```

The generated Compose file uses the paths expected by the CPA image:

```text
./cliproxyapi/config.yaml -> /CLIProxyAPI/config.yaml
./cliproxyapi/auths       -> /root/.cli-proxy-api
./cliproxyapi/logs        -> /CLIProxyAPI/logs
```

CPA hashes a plaintext `remote-management.secret-key` back into `cliproxyapi/config.yaml` on startup, so that file must remain writable.

CPAMP reads the CPA Management Key from a Docker secret and connects to CPA through the Docker internal URL:

```text
http://cli-proxy-api:8317
```

This connection is managed by `compose.yaml` and `secrets/cpa-management-key`, so setup proceeds directly to the CPAMP Admin Key step.

After deployment, open:

```text
http://<host>:18137/management.html
```

Fresh installs do not generate a CPAMP Admin Key. Startup logs contain a one-time bootstrap token; use it in the UI to set an Admin Key with at least 16 characters and at least three character classes. The demo client API key is only for a quick connectivity check; create named production clients in the panel.

:::

## CPAMP-Only Install

If CPA is already running, the installer probes `127.0.0.1:8317`. Selecting the detected service requires a CPA Management Key, and the installer must successfully request `/v0/management/config` before writing configuration.

If you connect an existing CPA now, the installer stores the validated connection in:

```text
.env
secrets/cpa-management-key
```

After startup, only set the CPAMP Admin Key in the UI. This is an installer-managed connection: CPA URL and CPA Management Key come from the install directory, and the panel cannot replace them directly. Update the files and restart CPAMP to change the connection.

If you decide later, the installer deploys Slim. Setup then lets you:

```text
- download the latest CPA selected through a signed manifest and verified SHA-256 checksum; or
- use an existing CPA after immediate URL and Management Key validation.
```

If you want the connection to be managed by files, choose the option that stores the CPA connection in local secret files. In that mode, CPA URL and CPA Management Key come from config files, and the panel cannot directly replace that connection.

For a same-host CPA, the installer validates `127.0.0.1:8317` from the host and writes this container address:

```text
http://host.docker.internal:8317
```

On Linux it also writes `host.docker.internal:host-gateway`, so the container can reach the host CPA process. If CPA runs on another machine, use that address instead.

## Native Package Mode

Native mode downloads the matching release asset: `_full` for bundled CPA and `_slim` for CPAMP only. The historical unsuffixed asset remains a Slim compatibility alias. The installer creates:

```text
runtime/<package>/
data/
run.sh
cpa-manager-plus.service  # Linux
cpa-manager-plus.log
cpa-manager-plus.pid
```

The native package is started in the background. On Linux the installer also creates `cpa-manager-plus.service`; copy it into your systemd service directory and enable it according to your host policy. On macOS, or with another process manager, keep using `run.sh` as the integration point.

::: details Automation, reruns, and repair

## Advanced Usage

Preview the plan without writing files or starting services:

```bash
CPAMP_DRY_RUN=1 bash install-cpamp.sh
```

Generate config but skip startup:

```bash
CPAMP_SKIP_EXECUTE=1 bash install-cpamp.sh
```

Non-interactive full Docker install:

```bash
CPAMP_NON_INTERACTIVE=1 \
CPAMP_CONFIRM=1 \
CPAMP_LANG=en-US \
CPAMP_INSTALL_MODE=stack \
CPAMP_DEPLOY_METHOD=docker \
CPAMP_INSTALL_DIR="$HOME/cpa-manager-plus" \
bash install-cpamp.sh
```

Common variables:

| Variable                    | Description                                                                                                          |
| --------------------------- | -------------------------------------------------------------------------------------------------------------------- |
| `CPAMP_LANG`                | `zh-CN` or `en-US`.                                                                                                  |
| `CPAMP_INSTALL_MODE`        | `stack` or `cpamp`.                                                                                                  |
| `CPAMP_DEPLOY_METHOD`       | `docker` or `native`.                                                                                                |
| `CPAMP_INSTALL_DIR`         | Install directory. Defaults to `~/cpa-manager-plus`.                                                                 |
| `CPAMP_API_PORT`            | Gateway port. Defaults to `8137`.                                                                                    |
| `CPAMP_PANEL_PORT`          | Recommended management entry. Defaults to `18137`.                                                                   |
| `CPAMP_PORT`                | Compatibility entry. Defaults to `18317`.                                                                            |
| `CPAMP_CPA_PORT`            | Public CPA port for full Docker install. Defaults to `8317`.                                                         |
| `CPAMP_IMAGE`               | CPAMP Docker image.                                                                                                  |
| `CPAMP_CPA_IMAGE`           | CPA Docker image.                                                                                                    |
| `CPAMP_VERSION`             | Native package version. Defaults to `latest`.                                                                        |
| `CPAMP_CPA_CONNECTION_MODE` | `setup` or `env`.                                                                                                    |
| `CPAMP_CPA_URL`             | CPA URL for `env` mode.                                                                                              |
| `CPAMP_CPA_MANAGEMENT_KEY`  | CPA Management Key for `env` mode.                                                                                   |
| `CPAMP_DETECTED_CPA_URL`    | Overrides the auto-detection candidate.                                                                              |
| `CPAMP_USE_DETECTED_CPA`    | Set to `1` in non-interactive mode to select and validate the detected CPA.                                          |
| `CPAMP_OPERATION`           | `install`, `upgrade`, `repair`, or `regenerate`. Existing non-interactive deployments require an explicit operation. |
| `CPAMP_PROJECT_NAME`        | Docker Compose project name. Defaults to `cpamp`; use another name for an isolated deployment on the same host.      |

## Rerun And Overwrite

`CPAMP_OPERATION` applies to Docker and installer-managed native deployments. Docker supports `upgrade`, `repair`, and `regenerate`; native deployments support `upgrade` and `regenerate`.

Before writing files, the installer checks both the install directory and Docker data volume. When it detects an existing deployment, interactive mode offers:

1. **Upgrade existing deployment**: pull and recreate containers without changing config or secrets. The installer records the current CPAMP image and the CPA image for split-stack installs. If an old image ID is missing, the local image no longer exists, or Compose uses a digest reference, it stops before pulling and asks you to restore the old service or explicitly regenerate the deployment. If the upgraded service never becomes healthy, it retags the saved image IDs, recreates the previous services, and verifies health again before reporting the failed upgrade.
2. **Repair admin login**: stop CPAMP, synchronize the SQLite admin credential with `secrets/cpamp-admin-key`, restart, and verify login. CPA and application data are not deleted.
3. **Regenerate deployment config**: back up generated config before replacing it while preserving secrets and the data volume by default. If you explicitly provide a new CPA URL and CPA Management Key, the installer validates the connection, backs up the old key, and atomically replaces `secrets/cpa-management-key`.
4. **Exit**.

Both `upgrade` and `regenerate` keep the existing deployment method, install scope, and CPA source. `regenerate` is not an in-place topology conversion between Full/Slim, Docker/native, or existing/bundled CPA. Use a new install directory for that conversion. For a parallel Docker deployment, also choose a new `CPAMP_PROJECT_NAME` and non-conflicting ports, verify the new deployment, and then migrate traffic and data.

When an older native deployment is detected, upgrade preserves `usage.sqlite`, WAL/SHM, `data.key`, CPA config/auths/logs, and the old runtime. Native archives are extracted into a staging directory before the version directory is replaced, so a failed download or extraction does not leave a half-written active package and the same target version can be retried. The installer writes startup files before stopping the old installer-managed PID. A failed health check restores the old `run.sh` and attempts to restart the previous version. If systemd or another external manager owns the listening process, the installer refuses to terminate an unverified PID and asks the operator to stop the service first.

An in-place Docker upgrade deliberately preserves an older custom `compose.yaml`. If that file only published legacy `18317`, the upgraded service remains reachable there but does not automatically expose `18137` or `8137`. After reviewing custom changes and the installer backup, run `CPAMP_OPERATION=regenerate` when you are ready to generate the new mappings.

Newly generated Compose files give CPAMP a 45-second stop grace period and separated CPA a 35-second grace period. Upgrade, repair, and rollback also run `docker compose stop -t 45` for preserved older Compose files, so migration does not require rewriting them first.

If the install directory was deleted but `cpamp_cpa-manager-plus-data` still exists, the installer no longer silently generates a new key and reports success. It requires either recovery of the old data or a fresh install with a different Compose project name.

Non-interactive upgrade:

```bash
CPAMP_OPERATION=upgrade \
CPAMP_NON_INTERACTIVE=1 \
CPAMP_CONFIRM=1 \
bash install-cpamp.sh
```

Non-interactive admin-login repair:

```bash
CPAMP_OPERATION=repair \
CPAMP_NON_INTERACTIVE=1 \
CPAMP_CONFIRM=1 \
bash install-cpamp.sh
```

If the install directory is gone and only the old Docker volume remains, non-interactive repair must also set the original `CPAMP_INSTALL_MODE=stack` or `CPAMP_INSTALL_MODE=cpamp` so the installer does not generate the wrong service combination.

To regenerate deployment config:

```bash
CPAMP_OPERATION=regenerate bash install-cpamp.sh
```

`CPAMP_OVERWRITE=1` remains compatible with the old workflow and maps to config regeneration. The installer backs up the previous `.env`, `.cpamp-native.env`, `compose.yaml`, CPA config, `run.sh`, service file, and existing `secrets/cpa-management-key` under `backups/installer-*`. You should still separately back up `secrets/`, `data/`, and `cliproxyapi/`. If `data.key` is lost, stored CPA Management Keys cannot be recovered.

:::

## Startup And Login Verification

After Docker installation, the script checks `18137/health`; ports `8137`, `18137`, and `18317` all reach the same gateway. Fresh installs proceed to UI setup without an Admin Key. Upgrades and repairs still validate a protected endpoint when a legacy admin secret exists.

If the container is healthy but the key is rejected, interactive mode offers to stop CPAMP and repair the database credential automatically. Non-interactive mode exits with a failure and instructs the operator to use `CPAMP_OPERATION=repair`. This prevents the installer from presenting a newly generated key that does not match an existing database.
