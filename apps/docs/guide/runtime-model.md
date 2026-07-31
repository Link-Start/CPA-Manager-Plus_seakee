# CPAMP 与 CPA 如何协作

CPA 是实际的模型路由引擎；CPAMP 完整模式把它和 Manager Server 统一放在一个 Gateway 后面，并保存请求历史、分析成本和处理账号状态。新用户只需要把 CPAMP 视为一个完整项目。

普通使用者只需要记住三件事：

1. 完整模式统一使用 `18137`：管理页面和模型 API 都从这个 Gateway 进入。
2. 管理登录使用 CPAMP 管理密钥；模型请求仍使用普通 CPA API Key。
3. 只有沿用 CPA 自己托管的轻量面板时，才直接打开 CPA 的 `:8317/management.html` 并使用 CPA Management Key。

## 请求应该发到哪里

| 操作                         | 地址                                             | 使用的密钥         |
| ---------------------------- | ------------------------------------------------ | ------------------ |
| 完整模式客户端请求模型       | CPAMP `:18137/v1/...` 等 Gateway 模型接口        | CPA 普通 API Key   |
| 使用 CPAMP 完整模式管理页面  | CPAMP `:18137/management.html` 或自定义 Base Path | CPAMP 管理密钥     |
| 使用 CPAMP 轻量面板          | CPA `:8317/management.html`                      | CPA Management Key |
| External / Slim 沿用已有 CPA | 初始化或配置中心填写的 CPA 地址                  | CPA Management Key |

不要把三类密钥混用。同一个 `18137` Gateway 会按请求路径分流：模型接口验证普通 CPA API Key，CPAMP 管理与分析接口验证 CPAMP 管理密钥。

## 两种模式的区别

- **轻量面板**：只替换 CPA 官方管理界面，不增加服务或数据库。
- **完整模式**：提供统一 Gateway、Manager Server 和本地 SQLite；Full 包内置 CPA，Slim 可以下载 CPA 或连接已有 CPA。

不知道应该使用哪一种时，查看[如何选择面板](./choosing-a-panel.md)。

::: details 请求和数据的完整路径

```text
Codex / Claude Code / 其他客户端
  -> CPAMP Gateway :18137
      -> 内置或已配置的 CPA
          -> 模型提供商
          -> 请求日志与用量队列

CPAMP Manager
  -> 管理 CPA 并读取用量事件
  -> 保存请求历史、价格、巡检和自动化状态
```

因此：

- 客户端请求失败时，先检查 Gateway、CPA、Provider、账号和客户端配置。
- CPAMP 页面没有数据时，先确认请求经过 CPA，再检查请求监控采集。
- 已有外部 CPA 的旧客户端可以继续直连 CPA，再逐步迁移到 `18137`；两条路径最终都由 CPA 处理模型请求。

:::

## 什么时候需要修改 CPA 配置

以下能力仍由 CPA 配置控制：

- Provider、模型路由、认证文件和 OAuth。
- 客户端 API 密钥、配额、日志和插件。
- 远程管理、用量发布和队列保留时间。

日常操作优先使用 CPAMP 页面。只有页面没有对应字段、部署前准备 CPA 或高级排障时，才需要直接编辑 CPA `config.yaml`。

## 下一步

- 安装：[快速开始](./getting-started.md)
- 添加模型服务：[AI 提供商](../manual/ai-providers.md)
- 配置客户端：[客户端接入](../gateway/clients.md)
- 监控没有数据：[请求监控为空](../troubleshooting/request-monitoring.md)
