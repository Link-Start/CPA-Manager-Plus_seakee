# How CPAMP Works With CPA

CPA is the model-routing engine. CPAMP Full Mode places it and Manager Server behind one Gateway while storing request history, analyzing cost, and maintaining account state. New users can treat CPAMP as one complete project.

Most users only need to remember three things:

1. Full Mode uses `18137` for both the management panel and model APIs.
2. Management login uses the CPAMP Admin Key, while model requests still use normal CPA API Keys.
3. Only the CPA-hosted Lightweight Panel uses CPA `:8317/management.html` and the CPA Management Key directly.

## Where Each Request Goes

| Action                            | Address                                                    | Key                |
| --------------------------------- | ---------------------------------------------------------- | ------------------ |
| Full Mode client model request    | CPAMP Gateway model paths such as `:18137/v1/...`          | CPA client API Key |
| Use CPAMP Full Mode management    | CPAMP `:18137/management.html` or a custom Base Path       | CPAMP Admin Key    |
| Use CPAMP Lightweight Panel       | CPA `:8317/management.html`                               | CPA Management Key |
| External / Slim uses existing CPA | CPA URL entered during initialization or configuration     | CPA Management Key |

Do not mix these credentials. The same `18137` Gateway routes by path: model APIs validate normal CPA API Keys, while CPAMP management and analytics APIs validate the CPAMP Admin Key.

## The Two Modes

- **Lightweight Panel**: replaces the official CPA management UI without adding a service or database.
- **Full Mode**: provides one Gateway, Manager Server, and local SQLite. Full packages bundle CPA; Slim can download CPA or connect to an existing CPA.

If you are unsure which one to use, read [Choosing A Panel](./choosing-a-panel.md).

::: details Complete request and data flow

```text
Codex / Claude Code / other clients
  -> CPAMP Gateway :18137
      -> bundled or configured CPA
          -> model providers
          -> request logs and usage queue

CPAMP Manager
  -> manages CPA and reads usage events
  -> stores request history, prices, inspection, and automation state
```

This means:

- For a failed client request, check the Gateway, CPA, the provider, the account, and client configuration first.
- When CPAMP has no data, confirm that requests pass through CPA, then inspect monitoring collection.
- Existing clients may keep reaching an external CPA directly during migration and move to `18137` gradually; CPA handles the model request on both paths.

:::

## When To Change CPA Configuration

CPA configuration still controls:

- Providers, model routing, auth files, and OAuth.
- Client API keys, quota, logs, and plugins.
- Remote management, usage publishing, and queue retention.

Prefer the CPAMP interface for daily work. Edit CPA `config.yaml` directly only when the UI does not expose a field, when preparing CPA before deployment, or during advanced troubleshooting.

## Next Steps

- Install: [Quick Start](./getting-started.md)
- Add model services: [AI Providers](../manual/ai-providers.md)
- Configure clients: [Client Configuration](../gateway/clients.md)
- Monitoring has no data: [Monitoring Has No Data](../troubleshooting/request-monitoring.md)
