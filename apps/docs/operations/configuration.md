# 配置与数据目录

CPAMP 的核心数据都在本地。部署时先搞清楚三件事：SQLite 放在哪里，`data.key` 怎么保存，管理员密钥从哪里来。

## 关键文件

| 文件               | 说明                                                  |
| ------------------ | ----------------------------------------------------- |
| `usage.sqlite`     | SQLite 数据库，保存请求事件、配置、价格、别名等数据。 |
| `usage.sqlite-wal` | SQLite WAL 文件，存在时必须一起备份。                 |
| `usage.sqlite-shm` | SQLite SHM 文件，存在时必须一起备份。                 |
| `data.key`         | 数据密钥，用于加密写入 SQLite 的敏感配置。            |

Docker 默认路径：

```text
/data/usage.sqlite
/data/data.key
```

原生包默认路径：

```text
./data/usage.sqlite
./data/data.key
```

## 管理员密钥

完整 Docker / 原生 Manager Server 模式使用 `cpamp_...` 管理员密钥登录。

新安装默认在 UI 中设置管理密钥。首次启动日志只输出一次性 bootstrap token；使用它进入初始化向导后，设置至少 16 位且包含四类字符中至少三类的管理密钥，也可以一键生成。

已有自动化或迁移部署仍可通过以下方式预置：

| 变量                         | 说明                   |
| ---------------------------- | ---------------------- |
| `CPA_MANAGER_ADMIN_KEY`      | 直接传入管理员密钥。   |
| `CPA_MANAGER_ADMIN_KEY_FILE` | 从文件读取管理员密钥。 |

不要把 bootstrap token 或管理密钥写入 URL、提交到仓库或粘贴到公开日志。bootstrap token 使用后即失效，并且有过期时间。

## CPA Management Key

CPA Management Key 用于访问 CPA 管理接口。

它的保存位置取决于配置来源：

- 通过 setup 或面板保存的 CPA 连接，会使用 `data.key` 加密后写入 SQLite。
- 通过安装器或环境变量管理的 CPA 连接，来自 `CPA_UPSTREAM_URL` 和 `CPA_MANAGEMENT_KEY` / `CPA_MANAGEMENT_KEY_FILE`。这种连接不写入 SQLite；如果使用一键安装脚本，密钥通常在安装目录的 `secrets/cpa-management-key`。
- Integrated Full 的内部 CPA Management Key 由 runtime 在数据目录中管理，初始化 UI 不要求用户填写或查看它。

CPAMP 轻量面板由 CPA 托管，浏览器持有 CPA Management Key，符合 CPA 端口访问方式。

## 管理入口 Base Path

默认入口是 `/management.html`。初始化完成后可在“系统 → 运行时与更新”动态修改为 `/`、`/admin`、`/panel` 等路径；保存成功后浏览器跳到新入口，旧入口立即返回 404。

也可以使用 `CPA_MANAGER_PANEL_BASE_PATH` 由环境变量锁定。环境变量管理时 UI 只读。Base Path 只降低可发现性，不是认证或访问控制；公网部署仍需 HTTPS、强管理密钥和必要的网络策略。

## 采集配置

推荐使用：

```text
USAGE_COLLECTOR_MODE=auto
```

自动模式会依次尝试 RESP Pub/Sub、HTTP queue 和 RESP pop。

约束：

- RESP 连接必须直连 CPA API 端口，通常是 `8317`。
- HTTP queue 可以经过 HTTP proxy。
- `pollIntervalMs` 不应超过 CPA 用量队列保留时间。
- CPA retention 默认 60s，最大 3600s。
- 同一个 CPA queue 只应由一个 Manager Server 消费。
