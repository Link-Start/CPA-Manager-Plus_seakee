# 客户端接入

完整模式用户只需要知道 CPAMP Gateway 地址和普通 CPA API Key。Codex、Claude Code、OpenCode 与 OpenAI SDK 都可以直接指向 `18137`；CPAMP 会把模型请求转发到内置或已配置的 CPA。

## 通用规则

| 项目 | 建议 |
|---|---|
| Base URL | 完整模式使用 CPAMP Gateway，例如 `https://gateway.example.com` 或 `http://localhost:18137`；轻量面板或兼容迁移可以继续直连 CPA `8317`。 |
| API 密钥 | 使用 CPA 的普通 API 密钥，不要使用 CPAMP 管理员密钥或 CPA Management Key。 |
| 模型 | 使用 CPA 提供商暴露的模型名或别名。 |
| 监控 | Gateway 最终把模型请求交给 CPA；CPA 发布用量事件后，CPAMP 才能在请求监控 / 用量分析中展示。 |

同域名反向代理时，把全部 HTTP 流量转发到 CPAMP `18137` 即可。Gateway 会按路径把 `/v1/*`、`/v1beta/*`、`/backend-api/codex/*` 等模型请求交给 CPA，并把 CPAMP 管理与分析请求交给 Manager Server。

## Codex

Codex 接入前先准备好：

- Base URL 指向 CPAMP `18137` 或其 HTTPS 反向代理地址。
- API 密钥使用 CPA 普通 API 密钥。
- Codex 提供商或认证文件已在 CPA 中配置。
- 如果要做账号巡检，认证文件中需要稳定的 `auth_index` 和可识别账号信息。

Codex 请求失败时，先看 [请求监控](../manual/monitoring.md) 的失败摘要；如果像账号或配额问题，再看 [Codex 账号巡检](../manual/codex-inspection.md)。

## Claude Code

Claude Code 接入前检查：

- CPA 中已有 Claude Code 提供商或兼容提供商。
- OAuth 或认证文件已完成。
- 客户端使用 CPAMP Gateway 地址和普通 CPA API Key。

OAuth 成功只能说明认证流程完成，不代表账号一定能服务请求。请求失败时，先看认证文件的账号状态，再看请求监控的失败摘要。

## OpenCode 与通用 OpenAI 客户端

OpenCode、OpenAI SDK 和多数中转客户端使用 OpenAI 兼容的 `/v1/...`：

```text
Base URL: https://gateway.example.com/v1
API 密钥: CPA 普通 API 密钥
模型:     CPA 暴露的模型名或别名
```

经过统一 Gateway 时，`/v1/models` 返回 401 通常是普通 CPA API Key 问题；`/status` 或 CPAMP 管理接口返回 401 才是 CPAMP 管理登录问题。

## Factory Droid / 其他工具

其他工具只要能填写 OpenAI 兼容或 Gemini 兼容端点，就可以使用 CPAMP Gateway 暴露的接口。Gateway 会确保请求经过 CPA；CPA 发布事件后，CPAMP 才能看到请求、成本和账号健康信息。
