# Configuration And Data Directory

CPAMP stores its core data locally. During deployment, identify three things first: where SQLite lives, how `data.key` is stored, and where the admin key comes from.

## Key Files

| File               | Description                                                                           |
| ------------------ | ------------------------------------------------------------------------------------- |
| `usage.sqlite`     | SQLite database for request events, configuration, prices, aliases, and related data. |
| `usage.sqlite-wal` | SQLite WAL file. Back it up when present.                                             |
| `usage.sqlite-shm` | SQLite SHM file. Back it up when present.                                             |
| `data.key`         | Data key used to encrypt sensitive configuration written to SQLite.                   |

Docker defaults:

```text
/data/usage.sqlite
/data/data.key
```

Native package defaults:

```text
./data/usage.sqlite
./data/data.key
```

## Admin Key

Full Docker and native Manager Server modes use a `cpamp_...` admin key for login.

New installs create the admin key in the UI. Startup logs print only a one-time bootstrap token. Use it to enter initialization, then set an admin key of at least 16 characters containing at least three of uppercase letters, lowercase letters, digits, and special characters. The UI can generate one.

Existing automation and migration deployments may still preconfigure it with:

| Variable                     | Description                     |
| ---------------------------- | ------------------------------- |
| `CPA_MANAGER_ADMIN_KEY`      | Pass the admin key directly.    |
| `CPA_MANAGER_ADMIN_KEY_FILE` | Read the admin key from a file. |

Do not place the bootstrap token or admin key in URLs, source control, or public logs. The bootstrap token expires and becomes invalid after use.

## CPA Management Key

CPAMP uses the CPA Management Key to access the CPA management API.

Where it is stored depends on the configuration source:

- CPA connections saved through setup or the panel are encrypted with `data.key` and written to SQLite.
- CPA connections managed by the installer or environment variables come from `CPA_UPSTREAM_URL` and `CPA_MANAGEMENT_KEY` / `CPA_MANAGEMENT_KEY_FILE`. That connection is not written to SQLite; with the one-click installer, the key is usually in `secrets/cpa-management-key` under the install directory.
- Integrated Full stores its internal CPA Management Key in the runtime data directory; initialization does not ask the user to enter or reveal it.

The CPAMP Lightweight Panel is hosted by CPA, and the browser holds the CPA Management Key, matching CPA-port access semantics.

## Management Base Path

The default entry is `/management.html`. After initialization, “System → Runtime & Updates” can dynamically change it to `/`, `/admin`, `/panel`, or another valid path. The browser moves to the new entry after save, and the old path immediately returns 404.

`CPA_MANAGER_PANEL_BASE_PATH` can lock the value from the environment. The UI is read-only when the environment owns it. A Base Path only reduces discoverability; public deployments still need HTTPS, a strong admin key, and appropriate network controls.

## Collection Configuration

Recommended setting:

```text
USAGE_COLLECTOR_MODE=auto
```

Auto mode tries RESP Pub/Sub, HTTP queue, and RESP pop in order.

Constraints:

- RESP connections must connect directly to the CPA API port, usually `8317`.
- HTTP queue can go through an HTTP proxy.
- `pollIntervalMs` should not exceed the CPA usage queue retention.
- CPA retention defaults to 60s and is capped at 3600s.
- Only one Manager Server should consume the same CPA queue.
