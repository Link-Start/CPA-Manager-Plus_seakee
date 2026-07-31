# 原生包部署

原生包适合不使用 Docker，或已经有 systemd、launchd、Windows 服务和其他进程管理方案的环境。新版同时提供 Full 与 Slim：

| 包 | 内容 | 首次初始化 |
| --- | --- | --- |
| `_full` | CPAMP + 内置 CPA | 只设置 CPAMP 管理密钥 |
| `_slim` | CPAMP，不预装 CPA | 在 UI 选择下载最新版 CPA或沿用已有 CPA，然后设置 CPAMP 管理密钥 |
| 无后缀旧包名 | Slim 兼容资产 | 与 `_slim` 相同，用于兼容旧下载链接 |

Full 与 Slim 都支持 Linux、macOS、Windows 的 amd64/arm64。

Linux Full 默认内置可在 glibc、musl 和较旧发行版上运行的 CPA `no-plugin` 便携构建。它不支持 CPA 动态库插件；如需动态库插件，请使用 Slim 并连接到单独部署的 glibc CPA。

推荐管理入口：

```text
http://<host>:18137/management.html
```

`18137`、`8137`、`18317` 由同一个 Gateway Handler 提供服务，能力完全一致。新部署只需要使用 `18137`。

## 推荐：一键安装器

```bash
curl -fsSLO https://raw.githubusercontent.com/seakee/CPA-Manager-Plus/main/bin/install-cpamp.sh
bash install-cpamp.sh
```

选择 Native 后：

- “CPA + CPAMP 完整安装”下载 `_full` 包。
- “仅安装 CPAMP”下载 `_slim` 包，并在首次 UI 中选择 CPA 来源。
- 安装器不会生成 CPAMP 管理密钥；启动后从日志取得一次性初始化令牌，在 UI 中设置管理密钥。
- 已有旧版 Native 安装会被识别为升级目标。`CPAMP_OPERATION=upgrade` 会保留 SQLite/WAL/SHM、`data.key`、CPA 配置/auths/logs 和旧 runtime，下载新包后再切换进程；新版本启动失败时恢复旧启动脚本并尝试重启旧版本。

默认安装器进程可以自动切换。若你已把 CPAMP 交给 systemd 或其他外部进程管理器，先停止该服务，再执行升级，随后按你的服务策略重新加载本地生成的 service 文件。

## 手动下载

从 [GitHub Releases](https://github.com/seakee/CPA-Manager-Plus/releases/latest) 下载对应平台资产。

Full 示例：

```text
cpa-manager-plus_<version>_linux_amd64_full.tar.gz
cpa-manager-plus_<version>_linux_arm64_full.tar.gz
cpa-manager-plus_<version>_darwin_amd64_full.tar.gz
cpa-manager-plus_<version>_darwin_arm64_full.tar.gz
cpa-manager-plus_<version>_windows_amd64_full.zip
cpa-manager-plus_<version>_windows_arm64_full.zip
```

Slim 示例：

```text
cpa-manager-plus_<version>_linux_amd64_slim.tar.gz
cpa-manager-plus_<version>_windows_arm64_slim.zip
```

Linux 架构映射：

```text
x86_64  -> amd64
aarch64 -> arm64
arm64   -> arm64
```

## 启动

macOS / Linux：

```bash
tar -xzf cpa-manager-plus_vX.Y.Z_linux_amd64_full.tar.gz
cd cpa-manager-plus_vX.Y.Z_linux_amd64_full
./cpa-manager-plusctl start
./cpa-manager-plusctl logs 100
```

Windows PowerShell：

```powershell
Expand-Archive .\cpa-manager-plus_vX.Y.Z_windows_amd64_full.zip -DestinationPath .
cd .\cpa-manager-plus_vX.Y.Z_windows_amd64_full
.\cpa-manager-plusctl.ps1 start
.\cpa-manager-plusctl.ps1 logs 100
```

包内 `.cpamp-runtime-mode` 会让控制脚本自动使用 `runtime` 子命令，并选择 `integrated` 或 `slim`。需要前台运行时：

```bash
./cpa-manager-plus runtime
```

Full 首次启动日志会显示一次性初始化令牌。Slim 下载内置 CPA 成功后会切换为 Integrated。

## 初始化向导

### Full

1. 输入一次性初始化令牌。
2. 设置 CPAMP 管理密钥，或一键生成。
3. 进入管理页面。

### Slim

1. 选择“下载最新兼容 CPA”或“沿用已有 CPA”。
2. 下载路径会验证签名清单、平台、架构、libc 和 SHA-256；已有 CPA 路径会立即验证地址和 Management Key。
3. 设置 CPAMP 管理密钥。
4. 进入管理页面。

管理密钥至少 16 位，并包含大写字母、小写字母、数字、特殊字符中的至少三类。

## 数据位置

默认数据目录为当前工作目录下的 `data/`：

```text
data/usage.sqlite
data/usage.sqlite-wal
data/usage.sqlite-shm
data/data.key
data/cpa/config.yaml
data/cpa/auths/
data/cpa/logs/
data/runtime/
```

可以固定到独立目录：

```bash
export CPA_MANAGER_RUNTIME_DATA_DIR=/var/lib/cpa-manager-plus
export USAGE_DATA_DIR=/var/lib/cpa-manager-plus
./cpa-manager-plusctl start
```

备份应包含完整数据目录。`data.key` 丢失后，SQLite 中加密保存的 CPA Management Key 无法恢复。

## systemd

使用一键安装器时，会在安装目录生成 `cpa-manager-plus.service`。确认路径和运行用户后复制到 systemd：

```bash
sudo cp ./cpa-manager-plus.service /etc/systemd/system/cpa-manager-plus.service
sudo systemctl daemon-reload
sudo systemctl enable --now cpa-manager-plus
```

手动包也可以直接让 systemd 调用控制脚本所在目录的二进制：

```ini
[Unit]
Description=CPA Manager Plus Runtime
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=cpa-manager-plus
Group=cpa-manager-plus
WorkingDirectory=/opt/cpa-manager-plus
Environment=CPA_MANAGER_RUNTIME_DATA_DIR=/var/lib/cpa-manager-plus
ExecStart=/opt/cpa-manager-plus/cpa-manager-plus runtime
Restart=on-failure
RestartSec=3

[Install]
WantedBy=multi-user.target
```

## 动态 Base Path

初始化后可在“系统 → 运行时与更新”把 `/management.html` 动态改为 `/`、`/admin`、`/panel` 等路径。无需重启；旧路径立即返回 404。

需要由服务配置锁定时：

```bash
export CPA_MANAGER_PANEL_BASE_PATH=/admin
./cpa-manager-plusctl start
```

环境变量配置时 UI 只读。Base Path 不是安全边界，公网部署仍需 HTTPS 和强管理密钥。

## 更新

- Full Integrated：在“系统 → 运行时与更新”检查并更新 CPAMP、CPA 或两者。失败时自动尝试回滚。
- Slim 下载 CPA 后：切换为 Integrated，后续同样使用 UI 更新。
- Slim 沿用已有 CPA：CPAMP 不托管外部 CPA 更新；更新外部 CPA 时遵循它自己的部署流程。
- 旧版 Native：先运行新版一键安装器执行一次平滑升级，之后 Integrated 部署可使用 UI 更新。

手动替换包时必须保留完整数据目录，不要覆盖 `usage.sqlite`、WAL/SHM、`data.key` 或 `data/cpa/`。

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

初始化错误会附带直达[初始化故障排查](../troubleshooting/setup.md)对应章节的链接。
