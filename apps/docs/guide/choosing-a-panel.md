---
title: 如何选择 CPA / CLIProxyAPI 管理面板
description: 对比官方 Management Center、CPAMP 轻量面板与 CPAMP 完整模式，选择适合 CPA 配置管理、请求监控、成本分析和账号运维的 WebUI。
---

# 如何选择 CPA / CLIProxyAPI 管理面板

CPAMP 完整模式已经把 CPA Gateway、管理和本地分析整合为一个项目，适合绝大多数新部署。已经运行 CPA、且只想替换它自带 UI 的用户仍可使用轻量面板。

## 快速结论

| 目标                                             | 推荐选择               |
| ------------------------------------------------ | ---------------------- |
| 使用 CPA 项目维护的上游原生 UI                   | 官方 Management Center |
| 不增加服务，用更清晰的 WebUI 替换官方界面        | CPAMP 轻量面板         |
| 全新安装，同时需要模型网关与管理能力             | CPAMP 完整模式         |
| 持久化请求历史、排查失败并分析成本               | CPAMP 完整模式         |
| 运行服务端账号巡检、配额冷却和账号处理自动化     | CPAMP 完整模式         |
| 先低成本使用，后续再决定是否部署 SQLite 和采集器 | 先用 CPAMP 轻量面板    |

## 官方 Management Center

官方面板是 CPA 项目维护的基准 WebUI，直接调用 CPA Management API，适合希望跟随上游默认界面的用户。

- 不需要额外服务。
- 从 CPA `:8317/management.html` 访问。
- 使用 CPA Management Key 登录。
- 管理配置、Provider、认证文件、OAuth、API Key、Quota 和日志。

官方项目：[`router-for-me/Cli-Proxy-API-Management-Center`](https://github.com/router-for-me/Cli-Proxy-API-Management-Center)。

## CPAMP 轻量面板

CPAMP 轻量面板使用同一个 CPA 端口和 Management API，用 CPAMP 的导航、表单、卡片与信息组织替代官方界面。

- 同样不需要 Manager Server、SQLite 或额外容器。
- 只需把 CPA 的 `panel-github-repository` 指向 CPA Manager Plus。
- 保留 CPA 原生的配置管理、认证文件、OAuth、Quota、日志和插件工作流。
- 适合不喜欢官方 UI，但仍希望保持单进程、单端口部署的用户。

查看 [CPAMP 轻量面板安装指南](../deployment/cpa-panel.md)。

## CPAMP 完整模式

完整模式提供统一 Gateway、Manager Server 和本地 SQLite。Full Docker / Full 原生包内置 CPA，Slim 可以下载最新版 CPA 或沿用已有 CPA。除管理能力外，它还提供轻量面板没有的服务端能力：

- 持久化 CPA usage queue 中的请求事件。
- 按账号、模型、Provider、API Key、项目和时间范围分析请求、成本、Token、延迟与失败。
- 保存模型价格、API Key 别名、巡检历史和自动化状态。
- 运行服务端 Codex/xAI 巡检、配额冷却和账号处理队列。
- 提供备份、后台迁移、性能诊断和独立 CPAMP 管理员登录。

## 入口与能力边界

| 使用方式               | 面板入口                                    | 适合谁                       |
| ---------------------- | ------------------------------------------- | ---------------------------- |
| 官方 Management Center | `http://<cpa-host>:8317/management.html`    | 希望继续使用上游原生 UI      |
| CPAMP 轻量面板         | `http://<cpa-host>:8317/management.html`    | 只想替换 UI，不增加服务      |
| CPAMP 完整模式         | `http://<cpamp-host>:18137/management.html` | 网关、监控、成本、巡检和自动化 |

CPA 托管的 `:8317` 面板不会连接或读取 Manager Server。完整能力从 CPAMP `:18137/management.html` 进入；同一个 `18137` Gateway 也可承接普通模型 API。

## 完整模式的安装方式

完整模式只有一套产品能力，可以选择两种安装方式和两种 CPA 来源：

- **Docker 部署（推荐）**：适合大多数新用户和服务器部署。
- **原生包部署**：适合不使用 Docker 的 Linux、macOS 或 Windows 主机。
- **Full**：内置 CPA，初始化时只设置 CPAMP 管理密钥。
- **Slim**：初始化时下载最新版 CPA，或验证并沿用已有 CPA。

两种安装方式都从 `:18137/management.html` 进入，并使用 CPAMP Admin Key 登录。

## 先体验再决定

[在线演示](https://seakee.github.io/CPA-Manager-Plus/)可以帮助你预览界面和操作结构，但它只使用虚构数据。演示站不是面板、部署方式或运行模式，不能连接、管理或监控真实 CPA，也不能用于验证实际功能是否正常。

## 推荐升级路径

1. 已有 CPA，只想更换 UI：安装 CPAMP 轻量面板。
2. 需要历史监控或成本分析：安装 Slim，并选择沿用已有 CPA。
3. 改为访问 `:18137/management.html`，完成初始化并逐步把客户端 Base URL 迁到同一个 Gateway。
4. CPA `8317` 轻量面板可以暂时保留，但它与完整模式的登录和分析能力相互独立。

## 下一步

- 安装 [CPAMP 轻量面板](../deployment/cpa-panel.md)。
- 查看 [能力矩阵](../reference/capability-matrix.md)。
- 使用推荐的 [Docker 部署](../deployment/docker.md)。
- 不使用 Docker 时安装 [原生包](../deployment/native.md)。
