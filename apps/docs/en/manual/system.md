# System

System shows what runtime the panel is connected to, which models are visible, and which basic details to collect before troubleshooting.

Use [Logs](./logs.md) for log content. Use [Monitoring](./monitoring.md), [Usage Analytics](./usage-analytics.md), and [Auth Files](./auth-files.md) for requests, cost, and account state.

## Main Content

- **Runtime management**: the top panel shows deployment mode, current CPAMP/CPA versions, available releases, notes, progress, failures, and rollback results.
- **Quick links**: main repository, Web UI repository, and documentation.
- **Model list**: available models from the current connection, grouped by source.
- **Clear login storage**: remove saved panel connection state from the browser.
- **Connection state**: confirm whether the panel is connected to a usable CPA or Manager Server.

The exact content depends on the usage option. The CPAMP Lightweight Panel connects to CPA, while Full Mode connects to Manager Server; Full Mode can be installed with Docker or a native package. The Live Demo only displays fictional data and is not a runtime mode that can connect to real services.

## Update Entry And UI Hierarchy

Dashboard keeps lightweight version checks and badges. Integrated/Full deployments with bundled CPA use the signed runtime manifest as the only source for badges and View Updates, avoiding conflicts between a public Release and an installable asset for the current platform. External and split deployments keep the public version check. When an installable CPAMP or CPA update is found, View Updates opens the runtime panel at the top of System.

System owns the complete workflow:

1. Verify the signed runtime manifest and show current and target versions for CPAMP and CPA.
2. Review component release notes before starting.
3. Track download, verification, installation, health-check, and switch progress.
4. Show failures and automatic rollback results. An operation token keeps status queries available while Manager restarts.

Managed updates are available only for Integrated/Full deployments with bundled CPA. Slim becomes Integrated after a successful CPA download. External CPA and installer-managed split deployments show their mode without update actions.

## Dynamic Base Path

Runtime management can dynamically move the frontend entry to `/`, `/admin`, `/panel`, or another safe absolute path. The new path takes effect immediately and the retired path returns 404. Reserved API paths, traversal, queries, and fragments are rejected.

When `CPA_MANAGER_PANEL_BASE_PATH` is set by the environment, the UI is read-only; update the deployment environment and restart. Base Path only reduces discoverability and does not replace TLS, a strong Admin Key, firewall rules, or access control.

## Model List

Use the model list to answer:

1. Which models the runtime currently exposes.
2. Which provider or configuration a model comes from.

If a client fails to request a model, first check whether the model is visible here. If it is not, inspect [AI Providers](./ai-providers.md) and model rules.

A visible model does not guarantee requests will succeed. Auth, quota, upstream state, and routing rules can still fail.

## Clear Login Storage

Clear login storage removes saved panel connection information from the current browser. Use it when:

- Switching to another CPAMP or CPA address.
- Admin credentials changed but the browser keeps using the old connection.
- Moving between local development, demo, and production environments.

This does not delete server configuration, auth files, SQLite data, or CPA configuration. It only affects the current browser.

## What To Collect For Troubleshooting

Before reporting a problem, record:

- CPAMP version and runtime mode.
- CPA version.
- Connection target: CPA, Manager Server, or a local development service.
- Whether the model list loads.
- Whether login storage was recently cleared or the address changed.

Then combine it with sanitized evidence from [Logs](./logs.md) and [Monitoring](./monitoring.md) for the same time window.
