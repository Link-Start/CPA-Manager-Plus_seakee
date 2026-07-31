---
title: 迁移到内置 CPA Runtime
description: 将旧版 CPAMP Docker 或原生部署平滑迁移到 Full/Slim、三端口网关、UI 初始化、动态 Base Path 和托管更新。
---

# 迁移到内置 CPA Runtime

本页适用于升级前已经运行 CPA Manager Plus 的用户。新版不会要求先重建部署：旧 `18317` 映射、SQLite、`data.key`、管理员凭证和已有 CPA 连接继续有效，再按需要逐步启用 `18137`、`8137`、内置 CPA 和动态 Base Path。

## 升级时如何识别旧部署

未显式设置 `CPA_MANAGER_DEPLOYMENT_MODE` 时，runtime 按以下顺序判断：

1. 存在 `CPA_UPSTREAM_URL`：按 installer-managed 分离部署启动，不启动内置 CPA。
2. SQLite 的 `manager_config_v1` 或旧 `setup` 中已有 CPA URL：按 external 连接沿用，不启动内置 CPA。
3. 没有旧 CPA 连接且包中存在 CPA 二进制：按 Integrated/Full 启动。
4. 没有 CPA 二进制：按 Slim 启动。

这一步只读检查旧 SQLite，不修改 `usage.sqlite`。Manager Server 随后继续使用原数据库完成兼容 schema 迁移。

## 必须一起保留的数据

升级、迁移和回滚前停止旧进程，并一起备份：

```text
usage.sqlite
usage.sqlite-wal
usage.sqlite-shm
data.key
```

分离式 CPA 还要保留：

```text
cliproxyapi/config.yaml
cliproxyapi/auths/
cliproxyapi/logs/
secrets/cpa-management-key
```

不要同时启动旧、新 Manager Server 消费同一个 CPA 队列。新版 runtime 会先占用全部网关和内部控制监听器，成功后才启动 Manager 与 CPA 子进程；端口冲突时不会先启动第二个采集器。

## Docker 平滑升级

1. 备份命名 volume 或宿主机数据目录。
2. 在原 Compose 目录执行 `docker compose pull && docker compose up -d`。
3. 先使用旧入口 `http://<host>:18317/management.html` 验证登录、历史数据和 CPA 连接。
4. 再为同一个容器增加端口映射：

```yaml
ports:
  - '8137:8137'
  - '18137:18137'
  - '18317:18317'
```

三个端口由同一个网关 Handler 提供相同 API 与代理能力。新文档以 `18137` 作为管理入口；旧客户端可以继续使用 `18317`，再逐步迁移。

使用新版安装器执行原位升级时，脚本会在拉取前确认旧镜像可用于回滚；镜像 ID 缺失、本地镜像不存在或 Compose 使用 digest 引用时会直接停止。新生成的 Compose 为 CPAMP 保留 45 秒、分离式 CPA 保留 35 秒停止宽限；旧 Compose 不会被强制重写，安装器会显式使用 45 秒停止窗口。

旧分离式 Compose 可以继续使用 CPA 容器和 `CPA_UPSTREAM_URL`。不要仅因为镜像现在包含 CPA 就删除旧 CPA 数据；只有明确迁移到 Integrated 后，才停止分离 CPA 并验证内置 CPA 的 config、auths、日志和 Management Key。

## 原生包迁移

- 使用一键安装器的旧版 Native 部署可直接重新运行新版脚本并选择“升级现有部署”，或设置 `CPAMP_OPERATION=upgrade`。脚本会在下载和生成新启动文件后才停止旧进程，并保留 SQLite/WAL/SHM、`data.key`、CPA config/auths/logs 与旧 runtime；新版本健康检查失败时恢复旧启动脚本并尝试重启旧版本。
- 想继续连接已有 CPA：下载 `_slim`（或兼容无后缀）资产，保持原 `USAGE_DATA_DIR` / `USAGE_DB_PATH`，由初始化或旧 SQLite 继续提供 CPA 连接。
- 想改为内置 CPA：下载 `_full` 资产，使用 `runtime` 模式，并在停机窗口验证内置 CPA 数据目录后再停用旧 CPA。
- 包内 `.cpamp-runtime-mode` 和控制脚本会选择 `slim` 或 `integrated`；显式传入 `serve` 仍可覆盖默认，但不会获得网关和托管更新能力。

如果旧进程已经交给 systemd 或其他外部进程管理器，先停止外部服务，再执行安装器升级并重新加载生成的 service 文件。安装器不会在 PID 不可确认时终止未知进程。

## 管理密钥与初始化

旧数据库中的 CPAMP 管理凭证继续有效，不会强制重新初始化。只有全新安装不再由安装器生成管理密钥，而是在 UI 中使用一次性 bootstrap token 设置。

旧 CPA 连接存在但没有 CPAMP 管理凭证时，初始化向导会跳过 CPA 步骤，只要求创建管理密钥。Slim 且没有 CPA 连接时，可选择下载最新版 CPA 或使用已有 CPA。

## Base Path 与反向代理

升级后默认入口仍是 `/management.html`。在 System 中改为 `/admin`、`/panel` 或 `/` 后，新入口立即生效，旧入口返回 404。先更新反向代理健康检查和路由，再切换公网入口；Base Path 不是安全认证机制。

## 更新能力

只有 Integrated/Full 部署可以在 System 中同时管理 CPAMP 与 CPA 更新。installer-managed 和 external 部署保留原升级流程。Slim 下载 CPA 成功后会转为 Integrated，并从此获得托管更新能力。

## 回滚

停止新版 runtime 后再恢复旧二进制或镜像，继续挂载同一备份数据。若新版已经写入 schema，优先恢复升级前的完整 SQLite + WAL/SHM + `data.key` 备份。不要让旧、新版本同时运行。
