# Quick Start

New users should start with CPAMP Full Mode: one Full deployment provides the gateway, management, and local analytics. Use the Lightweight Panel only when CPA already runs and you only want to replace its built-in management UI.

| Your situation                                                      | Recommended path                                             |
| ------------------------------------------------------------------- | ------------------------------------------------------------ |
| New deployment requiring gateway, management, and analytics         | [CPAMP Full Mode](#path-2-install-full-mode)                 |
| CPA already runs and you also need history, cost, or automation      | [CPAMP Full Mode](#path-2-install-full-mode)                 |
| CPA already runs and you only want a clearer management UI           | [CPAMP Lightweight Panel](#path-1-install-lightweight-panel) |
| You are not sure                                                     | Read [Choosing A Panel](./choosing-a-panel.md)               |

## Path 1: Install Lightweight Panel

Use this when CPA already runs and you only want to replace the official Management Center. It needs no additional service, database, or port.

1. Set CPA `panel-github-repository` to `seakee/CPA-Manager-Plus`.
2. Restart or reload CPA.
3. Open:

```text
http://<cpa-host>:8317/management.html
```

Log in with the CPA Management Key. See [Install Lightweight Panel](../deployment/cpa-panel.md) for the complete configuration and update instructions.

## Path 2: Install Full Mode

Full Mode provides one Gateway plus Manager Server, request history, cost analytics, server-side inspection, and automation. Full Docker and Full native packages bundle CPA. The installer is the recommended path for most users:

```bash
curl -fsSLO https://raw.githubusercontent.com/seakee/CPA-Manager-Plus/main/bin/install-cpamp.sh
bash install-cpamp.sh
```

In the installer:

1. Install scope: choose “CPA + CPAMP” if CPA is not installed, or “CPAMP only” if CPA already runs.
2. Deployment method: prefer Docker, or choose a native package when Docker is not used.
3. Review the summary and confirm deployment.

After installation, open:

```text
http://<host>:18137/management.html
```

Startup logs print a one-time bootstrap token. Use it to enter setup and create the CPAMP Admin Key in the UI; the installer does not generate or print that admin key. See [Install Full Mode](../deployment/installer.md) for details.

## First Initialization

- **Bundled CPA or installer-managed CPA + CPAMP**: enter the one-time bootstrap token and set the CPAMP Admin Key. No CPA URL or CPA Management Key is required.
- **Slim**: choose Download Latest Compatible CPA or Use Existing CPA. A successful download switches the deployment to Integrated; the existing-CPA path immediately validates the CPA URL and CPA Management Key.
- **CPAMP-only with an external CPA**: validate the CPA URL and CPA Management Key first, then set the CPAMP Admin Key.

The CPAMP Admin Key must be at least 16 characters and contain at least three of uppercase letters, lowercase letters, digits, and special characters. Setup includes a generate button.

## Confirm That It Works

### Lightweight Panel

- You can log in with the CPA Management Key.
- CPA management features such as providers, auth files, OAuth, quota, logs, and plugins load normally.
- The page remains on CPA `:8317/management.html`.

### Full Mode

- You can log in to `:18137/management.html` with the CPAMP Admin Key.
- Dashboard shows that CPA is connected.
- Monitoring shows an event after a real request passes through CPA.
- Usage Analytics shows the corresponding tokens and estimated cost.
- After creating a provider and normal CPA API Key, clients can use the same `18137` Gateway as their model API address.

If Full Mode opens but has no request data, see [Monitoring Has No Data](../troubleshooting/request-monitoring.md).

## What To Do Next

- Add model services: [AI Providers](../manual/ai-providers.md)
- Log in or import accounts: [OAuth Login](../manual/oauth.md) and [Auth Files](../manual/auth-files.md)
- Configure clients: [Client Configuration](../gateway/clients.md)
- Upgrade and back up: [Upgrade CPAMP](../operations/update.md) and [Backup And Restore](../operations/backup.md)
- Maintain deployment manually: [Docker Deployment](../deployment/docker.md) or [Native Packages](../deployment/native.md)
