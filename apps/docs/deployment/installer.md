# 一键安装脚本

安装脚本适合第一次部署，或已经有 CPA、只想把 CPAMP 跑起来的环境。它不会直接覆盖已有配置文件；执行前会先展示安装摘要，确认后才写入文件和启动服务。

大多数用户只需要完成四步：运行脚本、选择安装范围、选择 Docker 或原生包、确认摘要。安装完成后打开 `18137` 管理入口，在 UI 中设置 CPAMP 管理密钥。

## 运行方式

下载脚本后运行：

```bash
curl -fsSLO https://raw.githubusercontent.com/seakee/CPA-Manager-Plus/main/bin/install-cpamp.sh
bash install-cpamp.sh
```

如果需要先查看内容：

```bash
less install-cpamp.sh
bash install-cpamp.sh
```

脚本会按顺序处理：

1. 检查系统、架构、WSL、端口和必要命令。
2. 选择后续操作语言。
3. 自动探测本机已有 CPA；选择使用时，先验证 CPA Management Key。
4. 选择安装范围：CPA + CPAMP，或 Slim（仅 CPAMP）。
5. 选择部署方式：Docker 或原生包，并生成最小配置和必要的 CPA secret。
6. 展示摘要，可确认、返回修改或退出。
7. 确认后执行部署。

## 支持的组合

| 安装范围    | Docker | 原生包 |
| ----------- | -----: | -----: |
| CPA + CPAMP |   支持 |   支持 |
| 仅 CPAMP    |   支持 |   支持 |

原生 Release 同时提供 `_full` 与 `_slim` 包：`_full` 内置 CPA，`_slim` 只包含 CPAMP。Docker 只发布一个内置 CPA 的镜像；安装器选择“仅 CPAMP”时会让同一镜像以 Slim 模式运行且不启动内置 CPA。两种 Slim 都可以在初始化时下载当前最新版 CPA，或沿用已有 CPA。

## 完整 Docker 安装

没有现成 CPA 时选择这个组合。Docker 会使用分离的 CPA 与 CPAMP 容器；原生模式下载内置 CPA 的 `_full` 包。两种方式都把 CPAMP 管理密钥留在初始化 UI 中设置。

::: details 查看安装器生成的文件和连接方式

选择 CPA + CPAMP 后，脚本会生成：

```text
compose.yaml
.env
secrets/cpa-management-key
secrets/cpa-demo-client-key
cliproxyapi/config.yaml
cliproxyapi/auths/
cliproxyapi/logs/
```

默认生成的密钥格式如下：

```text
CPA Management Key: cpa_ + 32 位字母数字
演示客户端 API Key: sk- + 64 位字母数字
```

重跑脚本时，已有的非空单行 secret 文件会被原样复用；手动管理的密钥不需要符合默认生成格式。

CPA 最小配置会启用远程管理和用量发布：

```yaml
api-keys:
  - 'sk-...'

remote-management:
  secret-key: 'cpa_...'
  allow-remote: true

usage-statistics-enabled: true
redis-usage-queue-retention-seconds: 60
```

生成的 Compose 会按 CPA 镜像的实际工作目录挂载：

```text
./cliproxyapi/config.yaml -> /CLIProxyAPI/config.yaml
./cliproxyapi/auths       -> /root/.cli-proxy-api
./cliproxyapi/logs        -> /CLIProxyAPI/logs
```

CPA 启动时会把明文 `remote-management.secret-key` 自动写回为 bcrypt hash，所以 `cliproxyapi/config.yaml` 需要保持可写。

CPAMP 会通过 Docker secret 读取 CPA Management Key，并使用 Docker 内网地址：

```text
http://cli-proxy-api:8317
```

这组连接由安装目录中的 `compose.yaml` 和 `secrets/cpa-management-key` 管理，因此初始化向导会直接进入 CPAMP 管理密钥步骤。

部署完成后打开：

```text
http://<host>:18137/management.html
```

脚本不会为新安装生成 CPAMP 管理密钥。首次启动日志会输出一次性初始化令牌；在 UI 中使用该令牌设置至少 16 位、至少三类字符组合的管理密钥。演示客户端 API Key 只用于安装后快速连通性验证，生产客户端建议在面板里重新创建并按用途命名。

:::

## 仅安装 CPAMP

如果 CPA 已经在运行，安装器会探测 `127.0.0.1:8317`。选择使用检测结果后，必须输入 CPA Management Key；脚本会请求 `/v0/management/config`，只有验证成功才继续写配置。

选择“现在连接已有 CPA”后，脚本会把已验证连接写入安装目录：

```text
.env
secrets/cpa-management-key
```

启动后只需在 UI 中设置 CPAMP 管理密钥。这个模式是 installer-managed：CPA URL 和 CPA Management Key 来自安装目录，面板不能直接改写这组连接；需要调整时，更新安装目录中的配置和 secret 后重启 CPAMP。

如果选择稍后决定，安装器部署 Slim。打开初始化页后可以：

```text
- 下载经过签名 manifest 和 SHA-256 校验的最新版 CPA；
- 使用已有 CPA，并即时验证 CPA URL 和 CPA Management Key。
```

如果想让连接配置由环境管理，可以在脚本里选择“写入本机 secret 文件并由环境管理”。这种模式下，CPA URL 和 CPA Management Key 来自配置文件，面板不能直接改写这组连接。

Docker 方式连接同一宿主机的 CPA 时，脚本在宿主机使用 `127.0.0.1:8317` 验证，然后为容器写入：

```text
http://host.docker.internal:8317
```

Linux 上会同时写入 `host.docker.internal:host-gateway`，让容器能访问宿主机上的 CPA。CPA 跑在其他机器时，把 CPA URL 改成对应地址即可。

## 原生包模式

原生模式会按系统和架构下载 GitHub Release 资产：Full 使用 `_full`，Slim 使用 `_slim`（无后缀资产继续作为 Slim 兼容别名）。脚本生成：

```text
runtime/<package>/
data/
run.sh
cpa-manager-plus.service  # Linux
cpa-manager-plus.log
cpa-manager-plus.pid
```

原生包会以前台程序的方式启动到后台。Linux 会额外生成 `cpa-manager-plus.service`，可复制到 systemd 服务目录后按你的系统策略启用；macOS 或已有进程管理方式可以继续参考 `run.sh`。

::: details 自动化部署、重跑和修复

## 高级用法

只看计划，不写文件、不启动服务：

```bash
CPAMP_DRY_RUN=1 bash install-cpamp.sh
```

生成配置但不启动：

```bash
CPAMP_SKIP_EXECUTE=1 bash install-cpamp.sh
```

非交互完整 Docker 安装示例：

```bash
CPAMP_NON_INTERACTIVE=1 \
CPAMP_CONFIRM=1 \
CPAMP_LANG=zh-CN \
CPAMP_INSTALL_MODE=stack \
CPAMP_DEPLOY_METHOD=docker \
CPAMP_INSTALL_DIR="$HOME/cpa-manager-plus" \
bash install-cpamp.sh
```

常用变量：

| 变量                        | 说明                                                                               |
| --------------------------- | ---------------------------------------------------------------------------------- |
| `CPAMP_LANG`                | `zh-CN` 或 `en-US`。                                                               |
| `CPAMP_INSTALL_MODE`        | `stack` 或 `cpamp`。                                                               |
| `CPAMP_DEPLOY_METHOD`       | `docker` 或 `native`。                                                             |
| `CPAMP_INSTALL_DIR`         | 安装目录，默认 `~/cpa-manager-plus`。                                              |
| `CPAMP_API_PORT`            | 网关端口，默认 `8137`。                                                            |
| `CPAMP_PANEL_PORT`          | 推荐管理入口端口，默认 `18137`。                                                   |
| `CPAMP_PORT`                | 兼容入口端口，默认 `18317`。                                                       |
| `CPAMP_CPA_PORT`            | 完整 Docker 安装时 CPA 对外端口，默认 `8317`。                                     |
| `CPAMP_IMAGE`               | CPAMP Docker 镜像。                                                                |
| `CPAMP_CPA_IMAGE`           | CPA Docker 镜像。                                                                  |
| `CPAMP_VERSION`             | 原生包版本，默认 `latest`。                                                        |
| `CPAMP_CPA_CONNECTION_MODE` | `setup` 或 `env`。                                                                 |
| `CPAMP_CPA_URL`             | `env` 模式下的 CPA URL。                                                           |
| `CPAMP_CPA_MANAGEMENT_KEY`  | `env` 模式下的 CPA Management Key。                                                |
| `CPAMP_DETECTED_CPA_URL`    | 覆盖自动探测候选地址。                                                             |
| `CPAMP_USE_DETECTED_CPA`    | 非交互模式设为 `1` 时使用并验证检测到的 CPA。                                      |
| `CPAMP_OPERATION`           | `install`、`upgrade`、`repair` 或 `regenerate`。已有部署的非交互操作必须明确设置。 |
| `CPAMP_PROJECT_NAME`        | Docker Compose 项目名，默认 `cpamp`；需要在同一主机创建隔离的新部署时使用。        |

## 重跑和覆盖

`CPAMP_OPERATION` 同时覆盖 Docker 和安装器管理的原生部署。Docker 支持 `upgrade`、`repair`、`regenerate`；原生部署支持 `upgrade` 和 `regenerate`。

脚本会在写文件前检查安装目录和 Docker 数据卷。检测到已有部署时，交互模式会提供：

1. **升级现有部署**：只执行镜像拉取和容器更新，不修改配置或密钥。脚本会记录当前 CPAMP 镜像；分离式栈同时记录 CPA 镜像。旧镜像 ID 缺失、本地镜像不存在或 Compose 使用 digest 引用时，会在拉取前停止并要求先恢复旧服务或明确 regenerate。若新版服务始终无法通过健康检查，会重新标记旧镜像、重建旧服务并再次验证健康状态，然后再报告本次升级失败。
2. **修复管理员登录**：停止 CPAMP，把 SQLite 中的管理员凭证同步为 `secrets/cpamp-admin-key`，然后重启并验证登录；CPA 服务和业务数据不会被删除。
3. **重新生成配置**：备份现有生成配置后重新写入，默认继续复用 secret 和数据卷。若显式输入新的 CPA URL 与 CPA Management Key，脚本会先验证连接，再备份旧密钥并原子替换 `secrets/cpa-management-key`。
4. **退出**。

`upgrade` 和 `regenerate` 都保持已有部署的部署方式、安装范围和 CPA 来源；`regenerate` 不是 Full/Slim、Docker/native 或现有/内置 CPA 之间的拓扑转换入口。如需转换拓扑，请使用新的安装目录；Docker 并行部署还应使用新的 `CPAMP_PROJECT_NAME` 和不冲突的端口，验证新部署后再迁移流量和数据。

检测到旧版原生部署时，升级会保留 `usage.sqlite`、WAL/SHM、`data.key`、CPA 配置/auths/logs 和旧 runtime。原生包会先解压到临时目录，完整后才替换目标版本目录，因此下载或解压失败不会留下半写入的活动包，同一目标版本也可以再次重试。脚本生成新启动文件后再停止安装器 PID 管理的旧进程；健康检查失败时恢复旧 `run.sh` 并尝试重启旧版本。若 systemd 或其他外部进程管理器占用旧端口，脚本不会终止无法确认归属的进程，而是要求先手动停止服务。

Docker 原位升级会刻意保留旧版或自定义 `compose.yaml`。如果旧文件只发布了 `18317`，升级后仍可继续使用这个入口，但不会自动公开 `18137` 或 `8137`。确认自定义内容和安装器备份后，可在准备迁移端口时执行 `CPAMP_OPERATION=regenerate` 生成新映射。

新生成的 Compose 为 CPAMP 配置 45 秒停止宽限，分离式 CPA 配置 35 秒。升级、repair 和回滚旧 Compose 时，安装器也会显式执行 `docker compose stop -t 45`，因此不会要求用户先重写旧文件。

如果安装目录已经被删除、但 `cpamp_cpa-manager-plus-data` 仍然存在，脚本不会再静默创建新密钥并报告成功，而是要求恢复旧数据或使用新的 Compose 项目名进行全新安装。

非交互升级：

```bash
CPAMP_OPERATION=upgrade \
CPAMP_NON_INTERACTIVE=1 \
CPAMP_CONFIRM=1 \
bash install-cpamp.sh
```

非交互修复管理员登录：

```bash
CPAMP_OPERATION=repair \
CPAMP_NON_INTERACTIVE=1 \
CPAMP_CONFIRM=1 \
bash install-cpamp.sh
```

如果安装目录已经丢失、只剩旧 Docker 数据卷，非交互修复还必须设置原来的 `CPAMP_INSTALL_MODE=stack` 或 `CPAMP_INSTALL_MODE=cpamp`，避免生成错误的服务组合。

如果确定要重新生成配置：

```bash
CPAMP_OPERATION=regenerate bash install-cpamp.sh
```

`CPAMP_OVERWRITE=1` 继续兼容旧用法，并会映射到配置重新生成流程。脚本会把旧的 `.env`、`.cpamp-native.env`、`compose.yaml`、CPA 配置、`run.sh`、service 文件和现有 `secrets/cpa-management-key` 备份到安装目录的 `backups/installer-*`，但仍建议单独备份 `secrets/`、`data/` 和 `cliproxyapi/`。丢失 `data.key` 后，已保存的 CPA Management Key 无法恢复。

:::

## 启动和登录验证

Docker 安装后，脚本通过容器内的 `18137/health` 检查三个公开端口背后的同一网关。新安装没有管理员密钥，因此健康后进入 UI 初始化；旧部署升级或 repair 仍会在存在管理员 secret 时验证受保护接口。

如果容器已启动但密钥验证失败，交互模式会询问是否自动停止 CPAMP 并修复管理员凭证；非交互模式会返回失败状态，并提示使用 `CPAMP_OPERATION=repair`。这可以避免用户拿到一个与旧数据库不匹配的“新密钥”。
