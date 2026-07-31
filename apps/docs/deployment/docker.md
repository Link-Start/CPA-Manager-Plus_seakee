# Docker 部署

新版 CPAMP Full Docker 镜像已经内置 CPA。对新用户而言，CPAMP 是一个完整项目：安装一个容器后即可获得 CPA 网关基础能力、CPAMP 管理能力和本地数据分析能力，不需要先单独初始化 CPA。

推荐管理入口：

```text
http://<host>:18137/management.html
```

`18137`、`8137` 和兼容端口 `18317` 连接到同一个 Gateway Handler，API、管理接口和面板能力一致。新部署只需要公开 `18137`；`8137` 和 `18317` 用于现有客户端或旧部署平滑迁移。

## 选择部署方式

| 场景                                 | 建议                                                                  |
| ------------------------------------ | --------------------------------------------------------------------- |
| 全新安装，希望 CPAMP 自己管理 CPA    | 使用本页的单容器 Full Docker                                          |
| 使用一键安装脚本同时安装 CPA + CPAMP | 脚本继续生成分离式 Compose，便于保留现有 CPA 运维方式                 |
| 已有 CPA，只想连接它                 | 安装 Slim，首次打开面板时选择“沿用已有 CPA”                           |
| 已有旧版 Docker 部署                 | 保留原 `/data` 和 CPA 目录，按[更新指南](../operations/update.md)升级 |

## 最短安装流程

```yaml
services:
  cpa-manager-plus:
    image: seakee/cpa-manager-plus:latest
    restart: unless-stopped
    ports:
      - '18137:18137'
      # 可选兼容映射：
      # - '8137:8137'
      # - '18317:18317'
    environment:
      CPA_MANAGER_DEPLOYMENT_MODE: 'integrated'
      CPA_MANAGER_GATEWAY_ADDRS: '0.0.0.0:8137,0.0.0.0:18137,0.0.0.0:18317'
      USAGE_DB_PATH: '/data/usage.sqlite'
      CPA_MANAGER_DATA_KEY_PATH: '/data/data.key'
    volumes:
      - cpa-manager-plus-data:/data
    healthcheck:
      test: ['CMD', 'wget', '-qO-', 'http://127.0.0.1:18137/health']
      interval: 10s
      timeout: 3s
      retries: 3

volumes:
  cpa-manager-plus-data:
```

```bash
docker compose up -d
docker compose logs cpa-manager-plus
```

首次启动日志会显示一次性初始化令牌。打开管理入口后：

1. 输入一次性初始化令牌。
2. 设置至少 16 位、包含大写字母、小写字母、数字、特殊字符中至少三类的 CPAMP 管理密钥；也可以一键生成。
3. 完成初始化并进入管理页面。

内置 CPA 不需要填写 CPA 地址或 CPA Management Key。不要把一次性初始化令牌放进截图、工单或公开日志。

## `docker run`

```bash
docker run -d \
  --name cpa-manager-plus \
  --restart unless-stopped \
  -p 18137:18137 \
  -v cpa-manager-plus-data:/data \
  seakee/cpa-manager-plus:latest
```

如有旧客户端仍访问其他端口，可以同时增加：

```bash
-p 8137:8137 -p 18317:18317
```

三个端口能力一致，因此可以先增加 `18137`，再逐步把客户端和反向代理从旧端口切换过去。

## Slim：连接已有 CPA

需要继续使用已有 CPA 时，使用 Slim 原生包，或让一键安装器生成 CPAMP-only 部署。首次向导会提供两个选择：

- 下载最新兼容 CPA：校验签名清单和 SHA-256 后安装，成功后切换为 Integrated，并获得 CPAMP/CPA 托管更新能力。
- 沿用已有 CPA：填写 CPA 地址和 CPA Management Key，CPAMP 会立即验证 Management API，验证成功才进入下一步。

一键安装器生成的 Docker Full stack 仍采用分离式 CPA + CPAMP，部署模式为 `installer-managed`。它不会被单容器镜像强制切换为内置 CPA，也不会覆盖已有 CPA 配置、auths 或 logs。

## 自定义管理入口 Base Path

默认入口为 `/management.html`。初始化完成后可在“系统 → 运行时与更新”动态修改为 `/`、`/admin`、`/panel` 等路径，无需重启 CPAMP。切换成功后旧入口立即返回 404。

也可以在启动时固定：

```yaml
environment:
  CPA_MANAGER_PANEL_BASE_PATH: '/admin'
```

环境变量配置时 UI 只读。修改 Base Path 只能降低入口被随意发现的概率，不是认证或访问控制；公网部署仍应使用强管理密钥、HTTPS、防火墙或访问控制策略。

反向代理必须同时允许新路径。配置前先阅读[反向代理指南](./reverse-proxy.md)。

## 数据位置与备份

完整 `/data` 都应持久化。关键内容包括：

```text
/data/usage.sqlite
/data/usage.sqlite-wal
/data/usage.sqlite-shm
/data/data.key
/data/cpa/config.yaml
/data/cpa/auths/
/data/cpa/logs/
/data/runtime/
```

备份示例：

```bash
docker run --rm \
  -v cpa-manager-plus-data:/data \
  -v "$PWD":/backup \
  alpine \
  tar czf /backup/cpa-manager-plus-data-backup.tar.gz -C /data .
```

`data.key` 用于解密 SQLite 中保存的 CPA Management Key。丢失后只能重新配置 CPA 连接。备份或恢复时不要只复制 `usage.sqlite`，应同时保留 WAL/SHM 和 `data.key`。

## 更新

Integrated Full Docker 可以直接在 UI 中更新：

1. 仪表盘的版本卡片只做轻量检测，并在发现新版时链接到“系统”。
2. “系统 → 运行时与更新”展示 CPAMP/CPA 当前版本、Release Notes、兼容要求和更新范围。
3. 可选择仅更新 CPAMP、仅更新 CPA 或一起更新。
4. 更新过程显示进度；失败时自动尝试回滚，并可通过 operation token 在 Manager 短暂重启期间继续查询状态。

只有 Integrated 部署支持托管更新。分离式 `installer-managed`、External 和只连接已有 CPA 的 Slim 部署仍按[更新指南](../operations/update.md)更新镜像或外部 CPA。

正式 Release 镜像已经内置 runtime manifest 验签公钥。如果使用仓库中的 `docker-compose.manager.yml` 从源码执行 `--build`，需要通过构建变量提供 `CPA_MANAGER_RELEASE_PUBLIC_KEY` 才能启用托管更新；未配置时更新检测会安全失败，但网关、面板和分析能力不受影响。不要通过跳过签名校验来启用开发镜像更新。

即使启用 UI 自更新，也应定期更新基础镜像，以获得 Alpine、CA 证书和其他镜像层修复：

```bash
docker compose pull
docker compose up -d
```

## 常用环境变量

| 变量                          | 默认值                                     | 说明                                                                  |
| ----------------------------- | ------------------------------------------ | --------------------------------------------------------------------- |
| `CPA_MANAGER_DEPLOYMENT_MODE` | 自动推断                                   | Full Docker 设置为 `integrated`；旧部署会根据已有连接和内置二进制推断 |
| `CPA_MANAGER_GATEWAY_ADDRS`   | `0.0.0.0:8137,0.0.0.0:18137,0.0.0.0:18317` | 等价 Gateway 监听地址                                                 |
| `CPA_MANAGER_PANEL_BASE_PATH` | `/management.html`                         | 固定面板入口；设置后 UI 只读                                          |
| `USAGE_DATA_DIR`              | `/data`                                    | 持久化数据目录                                                        |
| `USAGE_DB_PATH`               | `/data/usage.sqlite`                       | SQLite 路径                                                           |
| `CPA_MANAGER_DATA_KEY_PATH`   | `/data/data.key`                           | 数据加密 key 路径                                                     |
| `CPA_UPSTREAM_URL`            | 空                                         | 连接外部 CPA；存在时旧部署推断为 `installer-managed`                  |
| `CPA_MANAGEMENT_KEY_FILE`     | 空                                         | 外部 CPA Management Key 文件                                          |

旧版的 `HTTP_ADDR` 仍用于直接运行 Manager Server 的高级场景；Integrated runtime 对外入口由 `CPA_MANAGER_GATEWAY_ADDRS` 管理。

## 验证

```bash
curl http://127.0.0.1:18137/health
curl http://127.0.0.1:18137/usage-service/info
curl http://127.0.0.1:18137/v1/models
```

初始化后：

```bash
curl -H "Authorization: Bearer <CPAMP_ADMIN_KEY>" \
  http://127.0.0.1:18137/status
```

如果初始化失败，错误提示中的“查看解决方案”会直达[初始化故障排查](../troubleshooting/setup.md)的对应锚点。
