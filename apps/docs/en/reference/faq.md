# FAQ

CPA Manager Plus provides full monitoring and analytics through the Manager Server-hosted panel. The old CPA-Manager workflow that attached an External Usage Service is not supported in Plus.

Use the recommended unified port for the Full Mode gateway, analytics, and monitoring:

```text
http://<host>:18137/management.html
```

## Which Usage Option Should I Choose?

| Goal                                                                            | Recommended mode                                      |
| ------------------------------------------------------------------------------- | ----------------------------------------------------- |
| New deployment requiring all features                                           | CPAMP Full Mode (Docker, recommended)                 |
| Request monitoring, historical statistics, model prices, aliases, import/export | CPAMP Full Mode                                       |
| Replace the official UI with a clearer interface and no additional service      | [CPAMP Lightweight Panel](../deployment/cpa-panel.md) |
| No Docker, but all features are required                                        | CPAMP Full Mode (native package)                      |

The CPAMP Lightweight Panel is an enhanced alternative to the official Management Center and is hosted directly by CPA. It does not configure Manager Server or read Manager Server SQLite data. Install Full or Slim and open `:18137/management.html` for the complete CPAMP feature set.

## Which Address Should I Open?

CPAMP Full Mode through Docker or a native package:

```text
http://<host>:18137/management.html
```

Gateway capabilities are identical on `18137`, `8137`, and `18317`; new deployments only need `18137`. The management Base Path can also be changed dynamically in “System → Runtime & Updates.”

The CPAMP Lightweight Panel is hosted by CPA and is normally accessed from the CPA port:

```text
http://<cpa-host>:8317/management.html
```

If you see login instead of setup, Manager Server is already configured. Use the CPAMP Admin Key to log in.

## What Is The Difference Between Admin Key And CPA Management Key?

| Place                                 | Key                                                |
| ------------------------------------- | -------------------------------------------------- |
| CPAMP Full / Slim management login             | CPAMP Admin Key, usually starting with `cpamp_...` |
| Integrated Full / installer-managed setup      | No user-entered CPA Management Key                 |
| External / Slim with an existing CPA            | CPA Management Key                                 |
| CPAMP Lightweight Panel login                   | CPA Management Key                                 |
| Normal model APIs and `GET /v1/models`           | CPA API Key                                        |
| CPAMP Manager Server APIs after initialization  | CPAMP Admin Key                                    |

Do not mix these keys. CPA connections saved through setup or the panel are encrypted with `data.key` and written to SQLite. Installer env/secret-managed connections read the key from the install directory and are not written to SQLite. In the CPAMP Lightweight Panel, the browser holds the CPA Management Key.

## Full Docker Opens Login Instead Of Initialization

The current data directory or environment already contains a CPAMP admin credential. A brand-new Full install without one shows bootstrap-token verification and CPAMP Admin Key creation.

Use the CPAMP Admin Key:

```text
cpamp_...
```

Do not use the CPA Management Key on the CPAMP login form. If the admin key is lost, follow [Reset Admin Key](../operations/reset-admin-key.md).

## What If I Forget The Admin Key?

Stop Manager Server, back up the data directory, then follow [Reset Admin Key](../operations/reset-admin-key.md).

## I Changed CPA Panel Repository, But Monitoring Is Empty

Changing the CPA panel repository only changes the frontend served by CPA.

Monitoring and historical analytics require Full Mode. To keep using this CPA, run the installer, choose CPAMP only, then choose Use Existing CPA during Slim initialization:

```bash
curl -fsSLO https://raw.githubusercontent.com/seakee/CPA-Manager-Plus/main/bin/install-cpamp.sh
bash install-cpamp.sh
```

Open:

```text
http://<host>:18137/management.html
```

During initialization:

```text
1. Enter the one-time bootstrap token
2. Choose Use Existing CPA
3. Enter and validate the CPA URL and CPA Management Key
4. Create the CPAMP Admin Key
```

Do not look for the old "External Usage Service" setting in Plus.

## Setup Shows The Wrong Default CPA URL

This only affects External mode or Slim's Use Existing CPA step. The suggested URL may come from the previous connection or frontend build configuration.

Fix options:

```text
1. Enter the correct CPA URL manually.
2. Rebuild the panel with VITE_DEFAULT_CPA_BASE_URL=<your-cpa-url>.
```

Docker Desktop host CPA commonly uses:

```text
http://host.docker.internal:8317
```

Same Compose network commonly uses:

```text
http://cli-proxy-api:8317
```

Linux host CPA + Docker CPAMP needs `--add-host=host.docker.internal:host-gateway`. See [Docker Deployment](../deployment/docker.md).

## What If The Monitoring Page Is Empty?

Start with [Request Monitoring Troubleshooting](../troubleshooting/request-monitoring.md) to inspect `/status`, collector, usage queue, and retention.

Common causes:

```text
1. CPA usage publishing is not enabled
2. Manager Server is not configured
3. CPA URL is wrong from the Manager Server network perspective
4. CPA Management Key is wrong
5. CPA version is too old
6. Collector mode is incompatible with the network path
7. Multiple Manager Servers consume the same CPA usage queue
8. Manager Server was down longer than CPA queue retention
9. Poll interval is longer than the queue retention window
```

Check CPA usage publishing:

```yaml
usage-statistics-enabled: true
```

Or enable it through CPA Management API:

```bash
curl -X PUT \
  -H "Authorization: Bearer <CPA_MANAGEMENT_KEY>" \
  -H "Content-Type: application/json" \
  -d '{"value":true}' \
  http://<cpa-address>:8317/v0/management/usage-statistics-enabled
```

Check Manager Server status:

```bash
curl -H "Authorization: Bearer <CPAMP_ADMIN_KEY>" \
  http://<cpamp-host>:18137/status
```

Important fields:

```text
collector.lastError
lastConsumedAt
lastInsertedAt
eventCount
```

If `lastConsumedAt` is empty, Manager Server is not consuming events. Check CPA URL, Management API, CPA version, queue publishing, and collector errors.

If `lastConsumedAt` changes but `lastInsertedAt` does not, SQLite writes are failing. Check disk space, permissions, and `/data` mount.

If both change but the page is empty, check browser filters, time range, hard-refresh the page, and verify `/v0/management/usage`.

## What Is `unsupported RESP prefix 'H'`?

This usually means a RESP collector connected to an HTTP endpoint.

RESP mode must directly reach the CPA API port. It cannot pass through a normal HTTP reverse proxy. Recommended fix:

```text
USAGE_COLLECTOR_MODE=auto
```

Use CPA `v6.10.8+` for the HTTP usage queue, or CPA `v7.1.39+` for the current recommended metadata set.

If you must use RESP, set CPA URL to a direct address:

```text
http://cli-proxy-api:8317
```

Do not use a public HTTPS reverse proxy domain.

## Can An HTTP Reverse Proxy Proxy RESP?

No. RESP Pub/Sub and RESP pop must connect directly to the CPA API port. HTTP queue can go through an HTTP proxy.

For same-domain deployment, forward all HTTP traffic to CPAMP `18137`; do not split model and management paths manually:

```text
HTTPS reverse proxy -> CPAMP :18137
  -> CPAMP management and analytics paths -> Manager Server
  -> model, CPA management, and callback paths -> bundled or configured CPA
```

Use the CPAMP Admin Key for CPAMP management paths and normal CPA API Keys for model paths. RESP Pub/Sub/pop still cannot pass through an HTTP reverse proxy; external RESP consumers must connect directly to the CPA API port.

## Container Cannot Connect To Host CPA

Inside a Docker container, `127.0.0.1` means the container itself.

If CPA runs on a Linux host and CPAMP runs in Docker:

```bash
docker run -d \
  --name cpa-manager-plus \
  --restart unless-stopped \
  --add-host=host.docker.internal:host-gateway \
  -p 18137:18137 \
  -e CPA_MANAGER_DEPLOYMENT_MODE=external \
  -v cpa-manager-plus-data:/data \
  seakee/cpa-manager-plus:latest
```

Then use:

```text
http://host.docker.internal:8317
```

Test from inside the container:

```bash
docker exec -it cpa-manager-plus sh
wget -qO- http://host.docker.internal:8317/healthz
```

## Docker Rebuild Lost Data

Most likely `/data` was not mounted, or Plus started with a new empty volume.

Correct:

```bash
-v cpa-manager-plus-data:/data
```

Wrong:

```bash
docker run seakee/cpa-manager-plus:latest
```

After migration from old CPA-Manager, be careful with volume names:

```text
old common volume: cpa-manager-data
Plus examples:     cpa-manager-plus-data
```

If you expect old data, mount the old volume or copy the old data.

## Why Do Backups Need data.key?

Back up the full data directory:

```text
usage.sqlite
usage.sqlite-wal
usage.sqlite-shm
data.key
```

CPA connections saved through setup or the panel are encrypted with `data.key` before being written to SQLite. If `data.key` is lost, the encrypted CPA Management Key cannot be recovered and the CPA connection must be saved again.

If the installer manages the connection through env/secrets, the CPA Management Key is usually stored in `secrets/cpa-management-key` under the install directory and is not written to SQLite. Back up `secrets/` together with the data directory.

## 401 From Manager Server

After setup, Manager Server endpoints require:

```text
Authorization: Bearer <CPAMP_ADMIN_KEY>
```

Examples:

```bash
curl -H "Authorization: Bearer <CPAMP_ADMIN_KEY>" \
  http://<cpamp-host>:18137/status
```

```bash
curl -H "Authorization: Bearer <CPAMP_ADMIN_KEY>" \
  http://<cpamp-host>:18137/v0/management/config
```

The CPA Management Key is not accepted for CPAMP Manager Server-only APIs.

## `/models` Returns 412

Manager Server setup is not complete.

Open:

```text
http://<cpamp-host>:18137/management.html
```

Finish setup first.

## Usage From A Downtime Window Is Missing

CPA usage queue is memory-backed and has limited retention.

Default retention:

```text
60 seconds
```

Maximum:

```text
3600 seconds
```

If Manager Server is down longer than the retention window, that period usually cannot be recovered from CPA. Keep Manager Server running continuously.

## CPAMP Lightweight Panel Is Missing Monitoring Or Model Prices

This is expected.

The CPAMP Lightweight Panel does not use Manager Server analytics and cannot attach to a separate Manager Server. Open:

```text
http://<cpamp-host>:18137/management.html
```

for monitoring, dashboard analytics, model prices, API key aliases, usage import/export, and server inspection.

## CPAMP Lightweight Panel Still Shows The Official Or An Older UI

Check that CPA configuration points at this project:

```yaml
remote-management:
  panel-github-repository: 'https://github.com/seakee/CPA-Manager-Plus'
```

If the new panel still does not load, clear CPA's cached panel file and reload or restart CPA:

```bash
rm static/management.html
```

If `Disable Panel Auto Updates` is enabled, CPA only downloads the panel again when the cached file is missing.

## Does CPAMP Upload Data?

No telemetry is uploaded. CPAMP does not include analytics SDKs and does not require a cloud account. By default it connects only to the CPA Gateway you configure; optional features such as model price sync, OAuth, and provider checks call external services only when you explicitly configure or trigger them.

## Does The Demo Site Connect To A Real Backend?

No. The live demo uses frontend mock data and does not need CPA, Manager Server, CPA Management Key, tokens, or SQLite.

## What Is management.html In A Release?

`management.html` is the single-file management panel shipped in release packages. Manager Server can host it, or CPA can load it as the CPAMP Lightweight Panel. The online documentation remains on GitHub Pages and is not distributed with install packages.
