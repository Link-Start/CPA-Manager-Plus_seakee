# Native Package Deployment

Native packages are for environments without Docker or with an existing systemd, launchd, Windows service, or other process manager. The new release provides both Full and Slim packages:

| Package | Contents | First initialization |
| --- | --- | --- |
| `_full` | CPAMP + bundled CPA | Set only the CPAMP Admin Key |
| `_slim` | CPAMP without preinstalled CPA | Choose the latest CPA or an existing CPA in the UI, then set the CPAMP Admin Key |
| Legacy unsuffixed name | Slim compatibility asset | Same behavior as `_slim`, retained for old download links |

Full and Slim are available for Linux, macOS, and Windows on amd64/arm64.

Linux Full bundles CPA's portable `no-plugin` build so the same package runs on glibc, musl, and older distributions. It does not support CPA dynamic-library plugins; use Slim with a separately deployed glibc CPA when those plugins are required.

Recommended management entry:

```text
http://<host>:18137/management.html
```

`18137`, `8137`, and `18317` use the same Gateway Handler and have identical capabilities. New deployments only need `18137`.

## Recommended: One-Click Installer

```bash
curl -fsSLO https://raw.githubusercontent.com/seakee/CPA-Manager-Plus/main/bin/install-cpamp.sh
bash install-cpamp.sh
```

After selecting Native:

- “CPA + CPAMP stack” downloads `_full`.
- “CPAMP only” downloads `_slim` and leaves CPA selection to the first-run UI.
- The installer no longer generates a CPAMP Admin Key. Read the one-time bootstrap token from the startup log and configure the Admin Key in the UI.
- Older native installs are detected as upgrade targets. `CPAMP_OPERATION=upgrade` preserves SQLite/WAL/SHM, `data.key`, CPA config/auths/logs, and the old runtime. It downloads the new package before switching processes; if startup fails, it restores the previous run script and attempts to restart the old version.

The installer can switch its default background process automatically. If CPAMP is managed by systemd or another external process manager, stop that service first, run the upgrade, then reload the generated service file according to your host policy.

## Manual Download

Download the matching asset from [GitHub Releases](https://github.com/seakee/CPA-Manager-Plus/releases/latest).

Full examples:

```text
cpa-manager-plus_<version>_linux_amd64_full.tar.gz
cpa-manager-plus_<version>_linux_arm64_full.tar.gz
cpa-manager-plus_<version>_darwin_amd64_full.tar.gz
cpa-manager-plus_<version>_darwin_arm64_full.tar.gz
cpa-manager-plus_<version>_windows_amd64_full.zip
cpa-manager-plus_<version>_windows_arm64_full.zip
```

Slim examples:

```text
cpa-manager-plus_<version>_linux_amd64_slim.tar.gz
cpa-manager-plus_<version>_windows_arm64_slim.zip
```

Linux architecture mapping:

```text
x86_64  -> amd64
aarch64 -> arm64
arm64   -> arm64
```

## Start

macOS / Linux:

```bash
tar -xzf cpa-manager-plus_vX.Y.Z_linux_amd64_full.tar.gz
cd cpa-manager-plus_vX.Y.Z_linux_amd64_full
./cpa-manager-plusctl start
./cpa-manager-plusctl logs 100
```

Windows PowerShell:

```powershell
Expand-Archive .\cpa-manager-plus_vX.Y.Z_windows_amd64_full.zip -DestinationPath .
cd .\cpa-manager-plus_vX.Y.Z_windows_amd64_full
.\cpa-manager-plusctl.ps1 start
.\cpa-manager-plusctl.ps1 logs 100
```

The package `.cpamp-runtime-mode` file makes the control script use the `runtime` subcommand and select `integrated` or `slim`. For foreground execution:

```bash
./cpa-manager-plus runtime
```

The first Full startup log prints a one-time bootstrap token. Slim switches to Integrated after downloading CPA successfully.

## Setup Wizard

### Full

1. Enter the one-time bootstrap token.
2. Set or generate the CPAMP Admin Key.
3. Enter the management panel.

### Slim

1. Choose “Download the latest compatible CPA” or “Use an existing CPA.”
2. The download path verifies the signed manifest, platform, architecture, libc, and SHA-256. The existing-CPA path validates the address and Management Key immediately.
3. Set the CPAMP Admin Key.
4. Enter the management panel.

The Admin Key must contain at least 16 characters and at least three of uppercase, lowercase, digits, and special characters.

## Data Location

The default data directory is `data/` under the current working directory:

```text
data/usage.sqlite
data/usage.sqlite-wal
data/usage.sqlite-shm
data/data.key
data/cpa/config.yaml
data/cpa/auths/
data/cpa/logs/
data/runtime/
```

Pin it to a separate directory when needed:

```bash
export CPA_MANAGER_RUNTIME_DATA_DIR=/var/lib/cpa-manager-plus
export USAGE_DATA_DIR=/var/lib/cpa-manager-plus
./cpa-manager-plusctl start
```

Back up the complete data directory. Without `data.key`, CPA Management Keys encrypted in SQLite cannot be recovered.

## systemd

The one-click installer writes `cpa-manager-plus.service` into the install directory. Review its paths and user, then install it:

```bash
sudo cp ./cpa-manager-plus.service /etc/systemd/system/cpa-manager-plus.service
sudo systemctl daemon-reload
sudo systemctl enable --now cpa-manager-plus
```

A manual package can run directly under systemd:

```ini
[Unit]
Description=CPA Manager Plus Runtime
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=cpa-manager-plus
Group=cpa-manager-plus
WorkingDirectory=/opt/cpa-manager-plus
Environment=CPA_MANAGER_RUNTIME_DATA_DIR=/var/lib/cpa-manager-plus
ExecStart=/opt/cpa-manager-plus/cpa-manager-plus runtime
Restart=on-failure
RestartSec=3

[Install]
WantedBy=multi-user.target
```

## Dynamic Base Path

After initialization, “System → Runtime & Updates” can change `/management.html` dynamically to `/`, `/admin`, `/panel`, or another valid path. No restart is required, and the old path returns 404 immediately.

To lock it through service configuration:

```bash
export CPA_MANAGER_PANEL_BASE_PATH=/admin
./cpa-manager-plusctl start
```

The UI is read-only when the environment controls the path. A Base Path is not a security boundary; public deployments still need HTTPS and a strong Admin Key.

## Updates

- Full Integrated: check and update CPAMP, CPA, or both under “System → Runtime & Updates.” Failures automatically trigger a rollback attempt.
- Slim after downloading CPA: switches to Integrated and uses the same UI update flow.
- Slim using an existing CPA: CPAMP does not manage updates for the external CPA; update it through its own deployment workflow.
- Older native versions: run the new one-click installer once for a smooth migration, then use managed UI updates for Integrated deployments.

When replacing packages manually, preserve the complete data directory. Do not overwrite `usage.sqlite`, WAL/SHM, `data.key`, or `data/cpa/`.

## Verification

```bash
curl http://127.0.0.1:18137/health
curl http://127.0.0.1:18137/usage-service/info
curl http://127.0.0.1:18137/v1/models
```

After initialization:

```bash
curl -H "Authorization: Bearer <CPAMP_ADMIN_KEY>" \
  http://127.0.0.1:18137/status
```

Setup errors include a direct link to the matching section in [Setup Troubleshooting](../troubleshooting/setup.md).
