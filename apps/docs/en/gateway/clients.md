# Client Configuration

Full Mode users only need the CPAMP Gateway address and a normal CPA API Key. Codex, Claude Code, OpenCode, and OpenAI SDKs can point directly at `18137`; CPAMP forwards model traffic to bundled or configured CPA.

## General Rules

| Item | Recommendation |
|---|---|
| Base URL | Use the CPAMP Gateway in Full Mode, such as `https://gateway.example.com` or `http://localhost:18137`. Lightweight Panel and migration deployments may keep using CPA `8317` directly. |
| API Key | Use a normal CPA API Key. Do not use the CPAMP Admin Key or CPA Management Key. |
| Model | Use the model name or alias exposed by CPA providers. |
| Monitoring | The Gateway ultimately sends model traffic through CPA. CPAMP displays events after CPA publishes them. |

With a same-domain reverse proxy, forward all HTTP traffic to CPAMP `18137`. The Gateway routes model paths such as `/v1/*`, `/v1beta/*`, and `/backend-api/codex/*` to CPA and routes CPAMP management and analytics paths to Manager Server.

## Codex

Before connecting Codex, prepare:

- Base URL pointing to CPAMP `18137` or its HTTPS reverse-proxy address.
- A normal CPA API Key.
- A Codex provider or auth file configured in CPA.
- Stable `auth_index` and recognizable account metadata if you want account inspection.

When Codex requests fail, start with the failure summary in [Monitoring](../manual/monitoring.md). If it looks like an account or quota issue, continue with [Codex Inspection](../manual/codex-inspection.md).

## Claude Code

Before connecting Claude Code:

- CPA has a Claude Code provider or compatible provider.
- OAuth or auth file setup is complete.
- The client uses the CPAMP Gateway address and a normal CPA API Key.

OAuth success only means authentication completed. If requests still fail, check account state in Auth Files first, then read the Monitoring failure summary.

## OpenCode And General OpenAI Clients

OpenCode, OpenAI SDKs, and most relay clients use the OpenAI-compatible `/v1/...` API:

```text
Base URL: https://gateway.example.com/v1
API Key:  normal CPA API Key
Model:    model name or alias exposed by CPA
```

Through the unified Gateway, a 401 from `/v1/models` is usually a normal CPA API Key problem. A 401 from `/status` or a CPAMP management endpoint is a CPAMP login problem.

## Factory Droid And Other Tools

Any tool that accepts an OpenAI-compatible or Gemini-compatible endpoint can use the API exposed by the CPAMP Gateway. The Gateway sends traffic through CPA; after CPA publishes events, CPAMP can observe requests, cost, and account health.
