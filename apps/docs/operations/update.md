# 更新 CPA Manager Plus 与 CPA

新版 CPAMP 提供两类更新路径：Integrated Full 由 UI 托管更新；分离式、External 和未集成 CPA 的 Slim 部署继续使用原有镜像、安装器或外部 CPA 更新流程。

## UI 层级

### 仪表盘：轻量检测入口

仪表盘版本卡片继续自动检测 CPAMP 和 CPA 版本。Integrated/Full 使用签名 runtime manifest，外部和分离式部署继续使用公开版本接口。这里的职责是快速提示：

- 当前版本与发现的新版本。
- 是否存在可用更新。
- Integrated 会显示“前往系统更新”，不直接在仪表盘执行变更。

把实际更新放在 System 页面，可以避免误触，并为 Release Notes、兼容要求、影响范围、进度和回滚留出完整空间。

### 系统 → 运行时与更新：完整更新流程

该区域显示：

- 部署模式和托管能力。
- CPAMP、CPA 当前版本与可用版本。
- Release 页面、Release Notes、CPA 最低 CPAMP 版本要求。
- “仅更新 CPAMP”“仅更新 CPA”“全部更新”。
- 更新确认、运行进度、组件结果、失败原因和回滚结果。

更新开始后，浏览器会保存一次性的 operation token。即使 CPAMP Manager 在二进制切换期间短暂不可用，页面仍可通过 Gateway runtime endpoint 查询本次操作；token 只允许读取对应 operation，不提供其他管理权限。

## 哪些部署支持托管更新

| 模式                                   | CPAMP 托管更新 | CPA 托管更新                  |
| -------------------------------------- | -------------- | ----------------------------- |
| Integrated Full Docker / Native        | 支持           | 支持                          |
| Slim 下载 CPA 并成功切换为 Integrated  | 支持           | 支持                          |
| Slim 沿用已有 CPA                      | 不支持         | 不支持，由外部 CPA 自己更新   |
| `installer-managed` 分离式 CPA + CPAMP | 不支持         | 不支持，使用 Compose 或安装器 |
| External / CPA Panel                   | 不支持         | 不支持                        |

托管更新会验证签名发布清单、平台/架构/libc、资产大小和 SHA-256。组件重启后必须通过健康检查；后续组件失败时，已经切换的组件会按相反顺序尝试回滚。

最新稳定版 CPAMP Release 上的清单每六小时自动刷新一次，也可以手动刷新。因此 CPA 独立发布后无需等待下一次 CPAMP 发版，就能出现在 UI 的更新检测中。如果仓库配置了 `CPA_RUNTIME_VERSION` 固定版本，Release 打包和定时清单刷新都会有意保持该 CPA 版本，直到固定值被修改。

签名清单会声明 CPA 所需的最低 CPAMP 版本。若当前 CPAMP 不满足要求，UI 会禁用“仅更新 CPA”；当清单中的最新版 CPAMP 可以满足要求时，应使用“全部更新”。如果当前发布通道中的 CPAMP 仍不满足要求，“全部更新”也会被禁用，需等待兼容版本进入该通道。定时清单刷新默认把当前稳定版 CPAMP 作为新 CPA 的保守兼容下限，因此旧版 CPAMP 可能需要一起更新。

停止预算按层级递增：CPA 最长约 30 秒，Supervisor 35 秒，Integrated runtime 40 秒，Docker CPAMP/安装器停止窗口 45 秒，Windows 原生控制脚本 50 秒。不要在自定义 Compose、systemd 或外部进程管理器中配置更短的停止宽限，否则正常优雅关闭可能被误判为失败并触发强杀或回滚。

## 更新前检查

1. 阅读目标版本 Release Notes 和兼容要求。
2. 备份完整数据目录：
   - `usage.sqlite`、`usage.sqlite-wal`、`usage.sqlite-shm`。
   - `data.key`。
   - `cpa/config.yaml`、`cpa/auths/`、`cpa/logs/`。
   - `runtime/`，用于保留已安装组件和回滚状态。
3. 确认没有第二个 Manager Server 使用同一 SQLite 或消费同一 CPA 用量队列。
4. 公网部署确认反向代理允许当前动态 Base Path。

## Integrated UI 更新

1. 打开“系统 → 运行时与更新”。
2. 点击“检查更新”。
3. 阅读两个组件的版本、Release Notes 和兼容要求。
4. 选择更新范围并确认。
5. 保持页面打开直到 operation 进入 `succeeded`、`failed` 或 `rolled_back`。

状态含义：

| 状态                 | 含义                                             |
| -------------------- | ------------------------------------------------ |
| `queued` / `running` | 正在验证、安装或重启组件                         |
| `handoff_pending`    | 新 CPAMP 已安装，正在重启运行时并等待新版本确认  |
| `rolling_back`       | 更新失败，正在恢复已切换组件                     |
| `succeeded`          | 所选组件已更新，且新运行时已确认服务健康         |
| `rolled_back`        | 更新失败，但旧版本已恢复                         |
| `failed`             | 更新或回滚未完全成功，需要查看组件错误并手动处理 |

Docker Integrated 更新后的二进制保存在 `/data/runtime/components/`。容器重启时，镜像内启动器会读取 runtime state 并委托给当前版本。仍应定期更新基础镜像，以获得系统层修复。

## 一键安装器生成的分离式 Docker

进入原安装目录：

```bash
cd "$HOME/cpa-manager-plus"
docker compose pull
docker compose up -d
docker compose ps
```

只更新 CPAMP：

```bash
docker compose pull cpa-manager-plus
docker compose up -d cpa-manager-plus
```

只更新 CPA：

```bash
docker compose pull cli-proxy-api
docker compose up -d cli-proxy-api
```

安装器升级不会重新生成 CPAMP 管理密钥，也不会覆盖 CPA `config.yaml`、auths、logs、Docker 数据卷或自定义 Compose 文件。重建服务前，脚本会记录当前 CPAMP 镜像；分离式栈同时记录 CPA 镜像。若无法读取旧镜像 ID、旧镜像已不存在，或 Compose 使用不可重新标记的 digest 引用，安装器会在 `pull` 之前失败关闭，避免进入无法自动回滚的状态。若新版部署始终无法健康启动，会重新标记这些旧镜像、在禁止拉取的情况下重建旧服务，并再次验证健康状态。需要修复或重新生成配置时使用安装器明确提供的 operation，不要把 `CPAMP_OVERWRITE=1` 当作普通升级方式。

## 手动 Docker

确认新容器继续挂载原来的 `/data` volume：

```bash
docker pull seakee/cpa-manager-plus:latest
docker stop cpa-manager-plus
docker rm cpa-manager-plus
docker run -d \
  --name cpa-manager-plus \
  --restart unless-stopped \
  -p 18137:18137 \
  -v cpa-manager-plus-data:/data \
  seakee/cpa-manager-plus:latest
```

如果旧客户端仍使用 `8137` 或 `18317`，继续映射这些端口；三个端口能力相同。

## 原生包

### 一键安装器管理的旧版 Native

重新下载新版安装器并选择升级，或非交互执行：

```bash
CPAMP_OPERATION=upgrade \
CPAMP_NON_INTERACTIVE=1 \
CPAMP_CONFIRM=1 \
bash install-cpamp.sh
```

脚本会识别旧 `run.sh`、PID/service 和 SQLite，保留数据、secret、CPA 状态和旧 runtime。压缩包会先解压到临时目录，完整后才替换目标版本目录，因此中断后可以使用同一版本重试。新包和启动文件准备完成后才停止旧进程；新版本未能健康启动时会恢复旧启动脚本并尝试重启旧版本。

systemd 或其他外部进程管理器不受安装器 PID 管理。先停止外部服务，执行升级，再重新安装或加载安装目录中的 service 文件。

### 手动替换

1. 停止进程。
2. 备份完整数据目录和旧包。
3. 下载匹配的 `_full` 或 `_slim` 包。
4. 继续使用原 `CPA_MANAGER_RUNTIME_DATA_DIR` / `USAGE_DATA_DIR`。
5. 启动新包并验证 `18137/health`。

不要复制空 `data/` 覆盖旧目录，也不要只复制 `usage.sqlite` 而漏掉 WAL/SHM 和 `data.key`。

## 验证与故障处理

```bash
curl http://127.0.0.1:18137/health
curl http://127.0.0.1:18137/usage-service/info
curl -H "Authorization: Bearer <CPAMP_ADMIN_KEY>" \
  http://127.0.0.1:18137/status
```

重点检查：

```text
deployment.mode
runtimeAvailable
components.cpamp
components.cpa
latestOperationId
operations
collector.lastError
```

常见问题会在 UI 返回具体错误码和文档链接。继续参考[初始化故障排查](../troubleshooting/setup.md)、[备份与恢复](./backup.md)和[集成运行时迁移](../migration/integrated-runtime.md)。
