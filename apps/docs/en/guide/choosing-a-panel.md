---
title: Choosing A CPA / CLIProxyAPI Management Panel
description: Compare the official Management Center, CPAMP Lightweight Panel, and CPAMP Full Mode for CPA administration, monitoring, cost analytics, and account operations.
---

# Choosing A CPA / CLIProxyAPI Management Panel

CPAMP Full Mode integrates the CPA Gateway, management, and local analytics as one project for most new deployments. Users who already run CPA and only want a replacement UI can keep using the Lightweight Panel.

## Quick Decision

| Goal                                                                   | Recommended choice         |
| ---------------------------------------------------------------------- | -------------------------- |
| Use the upstream UI maintained by the CPA project                      | Official Management Center |
| Replace the official UI with a clearer WebUI and no additional service | CPAMP Lightweight Panel    |
| New install requiring both model gateway and management                | CPAMP Full Mode            |
| Persist request history, diagnose failures, and analyze cost           | CPAMP Full Mode            |
| Run server-side inspection, quota cooldowns, and account automation    | CPAMP Full Mode            |
| Start with low commitment and decide about SQLite and collection later | CPAMP Lightweight Panel    |

## Official Management Center

The official panel is the baseline WebUI maintained by the CPA project. It talks directly to the CPA Management API and fits users who prefer the upstream default interface.

- No additional service is required.
- Open it from CPA at `:8317/management.html`.
- Log in with the CPA Management Key.
- Manage configuration, providers, auth files, OAuth, API keys, quota, and logs.

Official project: [`router-for-me/Cli-Proxy-API-Management-Center`](https://github.com/router-for-me/Cli-Proxy-API-Management-Center).

## CPAMP Lightweight Panel

The CPAMP Lightweight Panel uses the same CPA port and Management API while replacing the official interface with CPAMP navigation, forms, cards, and information hierarchy.

- It does not require Manager Server, SQLite, or another container.
- Point CPA `panel-github-repository` at CPA Manager Plus.
- Keep the native CPA configuration, auth file, OAuth, quota, log, and plugin workflows.
- Use it when you want a clearer interface while preserving a single-process, single-port deployment.

See [Install The CPAMP Lightweight Panel](../deployment/cpa-panel.md).

## CPAMP Full Mode

Full Mode provides one Gateway, Manager Server, and local SQLite. Full Docker/native packages bundle CPA, while Slim can download the latest CPA or use an existing CPA. It also provides server-backed capabilities that the Lightweight Panel does not:

- Persistent request events from the CPA usage queue.
- Request, cost, token, latency, and failure analytics by account, model, provider, API key, project, and time range.
- Saved model prices, API key aliases, inspection history, and automation state.
- Server-side Codex/xAI inspection, quota cooldowns, and the account action queue.
- Backups, background migrations, performance diagnostics, and an independent CPAMP Admin Key.

## Entry Points And Capability Boundary

| Usage option               | Panel entry                                 | Best for                                     |
| -------------------------- | ------------------------------------------- | -------------------------------------------- |
| Official Management Center | `http://<cpa-host>:8317/management.html`    | Keeping the upstream default UI              |
| CPAMP Lightweight Panel    | `http://<cpa-host>:8317/management.html`    | Replacing only the UI with no extra service  |
| CPAMP Full Mode            | `http://<cpamp-host>:18137/management.html` | Gateway, monitoring, cost, inspection, and automation |

The CPA-hosted `:8317` panel does not connect to or read Manager Server. Open CPAMP `:18137/management.html` for Full Mode; the same `18137` Gateway can also serve normal model APIs.

## Full Mode Installation Options

Full Mode has one product capability set with two installation choices and two CPA sources:

- **Docker deployment (recommended)**: for most new users and server deployments.
- **Native package deployment**: for Linux, macOS, or Windows hosts without Docker.
- **Full**: bundles CPA, so setup only creates the CPAMP Admin Key.
- **Slim**: downloads the latest CPA during setup or validates and keeps an existing CPA.

Both installation methods use `:18137/management.html` and the CPAMP Admin Key.

## Preview Before Choosing

The [Live Demo](https://seakee.github.io/CPA-Manager-Plus/) lets you preview the interface and navigation with fictional data. It is not a panel, deployment option, or runtime mode. It cannot connect to, manage, or monitor a real CPA instance and cannot validate whether real features are working.

## Recommended Upgrade Path

1. Already run CPA and only want a different UI: install the CPAMP Lightweight Panel.
2. Need historical monitoring or cost analytics: install Slim and choose the existing CPA.
3. Switch to `:18137/management.html`, finish initialization, and gradually move client base URLs to the same Gateway.
4. The CPA `8317` Lightweight Panel may remain during migration, but its login and analytics capabilities are independent from Full Mode.

## Next Steps

- Install the [CPAMP Lightweight Panel](../deployment/cpa-panel.md).
- Review the [Capability Matrix](../reference/capability-matrix.md).
- Use the recommended [Docker Deployment](../deployment/docker.md).
- Use [Native Packages](../deployment/native.md) without Docker.
