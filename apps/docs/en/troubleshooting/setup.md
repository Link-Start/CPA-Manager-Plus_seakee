---
title: Setup And Runtime Troubleshooting
description: Resolve CPA connection, management-key, bootstrap-token, Base Path, Slim provisioning, and managed-update errors by setup error code.
---

# Setup And Runtime Troubleshooting

The setup UI links each error to the matching section below. Keep the displayed error code, but never publish a CPA Management Key, CPAMP Admin Key, or one-time bootstrap token.

<a id="cpa-connection-required"></a>

## CPA Address Or Management Key Is Missing

This error should appear only in External mode or Slim's Use Existing CPA step. Enter the existing CPA URL and CPA Management Key. Use that CPA API address, such as `http://127.0.0.1:8317`; do not configure the unified `18137` Gateway as its own upstream.

<a id="setup-managed-by-environment"></a>

## The CPA Connection Is Environment-Managed

When the installer connects an existing CPA, it stores the connection in `.env` and `secrets/cpa-management-key` under the install directory. Update those files and restart CPAMP; the UI does not overwrite environment-managed values.

<a id="cpa-management-key-invalid"></a>

## The CPA Management Key Is Invalid

Use CPA's `remote-management.secret-key`, not a client API key or CPAMP Admin Key. Confirm remote management is enabled and test `/v0/management/config` again from the CPAMP host.

<a id="cpa-management-api-unreachable"></a>

## The CPA Management API Is Unreachable

Check the CPA process, URL, port, firewall, TLS certificate, and reverse proxy. A Docker container reaches a host CPA through `host.docker.internal`; Linux Compose also needs the `host-gateway` mapping.

<a id="cpa-usage-config-unavailable"></a>

## CPAMP Cannot Read The CPA Usage Configuration

CPAMP reached CPA but could not read the usage-queue settings from `/v0/management/config`. Confirm that the CPA version supports the Management API, that it returns valid JSON, and that a reverse proxy is not caching, truncating, or rewriting the response.

<a id="cpa-usage-retention-invalid"></a>

## The CPA Usage Queue Retention Is Invalid

Set CPA's `redis-usage-queue-retention-seconds` to a value greater than `0`, then restart CPA. The default `60` seconds is recommended. A busier or higher-latency host can use a larger value within the limit supported by its CPA version.

<a id="poll-interval-exceeds-retention"></a>

## The CPAMP Poll Interval Exceeds CPA Retention

CPAMP's `pollIntervalMs` must be less than or equal to `redis-usage-queue-retention-seconds × 1000`. Reduce the poll interval or increase CPA retention, then retry setup.

<a id="enable-cpa-usage-statistics-failed"></a>

## CPAMP Could Not Enable CPA Usage Statistics

After validating the connection, CPAMP could not enable usage statistics through `/v0/management/usage-statistics-enabled`. Confirm that the CPA version supports this endpoint, the remote Management Key has write access, and the reverse proxy allows `PUT`. Retry setup after fixing it; CPAMP does not persist the local setup when this step fails.

<a id="admin-key-policy"></a>

## The CPAMP Admin Key Does Not Meet Policy

Use at least 16 characters and at least three of uppercase letters, lowercase letters, digits, and symbols. The setup page can generate one securely.

<a id="admin-key-too-short"></a>

## The CPAMP Admin Key Is Too Short

Choose a new key with at least 16 characters and submit it again.

<a id="admin-key-already-initialized"></a>

## The Admin Key Is Already Initialized

Refresh and log in with the existing CPAMP Admin Key. If it is lost, follow [Reset Admin Key](../operations/reset-admin-key.md).

<a id="admin-key-invalid"></a>

## The Current CPAMP Admin Key Is Invalid

Confirm you opened the CPAMP management entry rather than CPA's native panel. Use the admin-key repair workflow if login still fails.

<a id="admin-verification-busy"></a>

## Admin Key Verification Is Busy

Retry after one second. This protection means multiple expensive key checks are already running and prevents unauthenticated requests from consuming unbounded CPU. If it persists, inspect the public entry for invalid-login traffic and add connection or request rate limits at the reverse proxy.

<a id="bootstrap-token-unavailable"></a>

## No One-Time Bootstrap Token Is Available

Restart CPAMP to issue a new token, then find `one-time bootstrap token` in Docker or native logs. The token is only for first-time setup.

<a id="bootstrap-token-expired"></a>

## The One-Time Bootstrap Token Expired

Restart CPAMP and use the newly logged token.

<a id="bootstrap-token-invalid"></a>

## The One-Time Bootstrap Token Is Invalid

Copy the complete token from the latest startup log without spaces or log prefixes. A restart invalidates the previous token.

<a id="slim-cpa-source-invalid"></a>

## The Slim CPA Source Is Invalid

Refresh setup and select either Download Latest CPA or Use Existing CPA.

<a id="slim-cpa-provision-unavailable"></a>

## This Deployment Cannot Select A CPA Source

Only a Slim package can download CPA or switch to an existing CPA during setup. External, installer-managed, and already integrated deployments do not expose these actions.

<a id="slim-cpa-download-failed"></a>

## Slim Could Not Download Or Start CPA

Check GitHub Release access, the configured signing public key, disk space, and whether a matching platform/libc asset exists. CPAMP data remains intact after a failed attempt.

<a id="slim-transition-recovery-pending"></a>

## The Slim Transition Is Waiting For Recovery

CPAMP persisted the CPA connection, but Runtime Control could not finish the final commit. Confirm that CPA is reachable, its Management Key is still valid, and review the Runtime Control log. Refresh setup or restart CPAMP after fixing the cause; reconciliation is idempotent and does not require redeployment.

<a id="panel-base-path-environment"></a>

## Base Path Is Environment-Managed

Remove or update `CPA_MANAGER_PANEL_BASE_PATH` and restart. The UI cannot dynamically override an environment-owned path.

<a id="panel-base-path-invalid"></a>

## Base Path Is Invalid

Use an absolute path such as `/`, `/admin`, or `/panel`. Do not use queries, fragments, backslashes, `.`/`..`, or reserved API paths such as `/health`, `/setup`, `/v0`, and `/v1`.

<a id="runtime-update-not-managed"></a>

## Managed Updates Are Not Available

CPAMP manages CPAMP and CPA updates only for Integrated/Full deployments with bundled CPA. External CPA and installer-managed split deployments keep their existing upgrade workflow.

<a id="runtime-update-check-failed"></a>

## Update Check Failed

Check the runtime manifest URL, network, system clock, and release signing public key. Do not bypass signature or checksum verification.

<a id="runtime-update-start-failed"></a>

## The Update Task Could Not Start

Confirm that no update is already running, the data directory is writable, and disk space is available. Review the task detail and service log.

<a id="runtime-operation-not-found"></a>

## The Update Operation Is Missing

Refresh System to load the latest task. Old operations may have been pruned, or the query token may belong to another CPAMP instance.

<a id="runtime-control-unavailable"></a>

## CPAMP Runtime Control Is Unavailable

Confirm the service starts in `runtime` mode, the internal control port is free, and the Manager child receives the runtime control address and key.
