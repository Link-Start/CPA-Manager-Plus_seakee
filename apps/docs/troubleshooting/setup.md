---
title: 初始化与运行时错误排查
description: 按初始化错误码定位 CPA 连接、管理密钥、一次性令牌、Base Path、Slim 下载和运行时更新问题。
---

# 初始化与运行时错误排查

初始化页面中的“查看解决方案”会直接跳到本页对应条目。先保留页面显示的错误码，但不要公开 CPA Management Key、CPAMP 管理密钥或一次性初始化令牌。

<a id="cpa-connection-required"></a>

## CPA 地址或管理密钥缺失

这个错误只应出现在 External 或 Slim“沿用已有 CPA”的连接步骤。填写完整的 CPA 地址和 CPA Management Key；地址应指向那台已有 CPA 的 API，例如 `http://127.0.0.1:8317`，不要把统一 Gateway 的 `18137` 回填为它自己的上游。

<a id="setup-managed-by-environment"></a>

## CPA 连接由安装环境管理

一键安装器使用已有 CPA 时，会把连接保存在安装目录的 `.env` 和 `secrets/cpa-management-key`。请修改这些文件并重启 CPAMP；UI 不会覆盖环境管理的连接。

<a id="cpa-management-key-invalid"></a>

## CPA Management Key 无效

确认使用的是 CPA `remote-management.secret-key`，不是客户端 API Key 或 CPAMP 管理密钥。确保 CPA 已启用远程管理，并从 CPAMP 主机重新请求 `/v0/management/config` 验证。

<a id="cpa-management-api-unreachable"></a>

## 无法访问 CPA Management API

依次检查 CPA 进程、地址、端口、防火墙、HTTPS 证书和反向代理。Docker 连接宿主机 CPA 时应使用 `host.docker.internal`；Linux Compose 还需要 `host-gateway` 映射。

<a id="cpa-usage-config-unavailable"></a>

## 无法读取 CPA 用量配置

CPAMP 已连接到 CPA，但无法读取 `/v0/management/config` 中的用量队列配置。确认 CPA 版本支持 Management API、响应是有效 JSON，并检查反向代理没有缓存、截断或改写该响应。

<a id="cpa-usage-retention-invalid"></a>

## CPA 用量队列保留时间无效

将 CPA 的 `redis-usage-queue-retention-seconds` 设置为大于 `0` 的值并重启 CPA。推荐保留默认 `60` 秒；如果网络或主机负载较高，可适当增加，但最大值仍受 CPA 版本限制。

<a id="poll-interval-exceeds-retention"></a>

## CPAMP 读取间隔超过 CPA 保留时间

CPAMP 的 `pollIntervalMs` 必须小于或等于 CPA 的 `redis-usage-queue-retention-seconds × 1000`。缩短读取间隔，或增加 CPA 的保留时间后重新初始化。

<a id="enable-cpa-usage-statistics-failed"></a>

## 无法开启 CPA 用量统计

CPAMP 在连接验证后无法通过 `/v0/management/usage-statistics-enabled` 开启用量统计。确认 CPA 版本支持该接口、远程管理密钥拥有写入权限，并检查反向代理允许 `PUT` 请求。修复后重试初始化；本地初始化配置不会在该步骤失败时写入。

<a id="admin-key-policy"></a>

## CPAMP 管理密钥不符合策略

密钥至少 16 位，并在大写字母、小写字母、数字、特殊字符四类中至少包含三类。可以使用初始化页的“一键生成”。

<a id="admin-key-too-short"></a>

## CPAMP 管理密钥长度不足

使用至少 16 位的新密钥，然后重新提交。

<a id="admin-key-already-initialized"></a>

## 管理密钥已经初始化

刷新页面并使用现有 CPAMP 管理密钥登录。若密钥丢失，按[重置管理员密钥](../operations/reset-admin-key.md)处理。

<a id="admin-key-invalid"></a>

## 当前 CPAMP 管理密钥无效

确认打开的是 CPAMP 管理入口，而不是 CPA 原生面板。仍无法登录时执行管理员密钥修复流程。

<a id="admin-verification-busy"></a>

## 管理密钥校验繁忙

等待一秒后重试。该保护表示当前已有多个高成本密钥校验正在执行，用于限制未认证请求占用 CPU。如果持续出现，请检查公网入口是否存在大量无效登录请求，并在反向代理层增加连接或请求速率限制。

<a id="bootstrap-token-unavailable"></a>

## 一次性初始化令牌不可用

重启 CPAMP 以签发新令牌，然后从 Docker 或原生日志中查找 `one-time bootstrap token`。令牌只用于首次初始化。

<a id="bootstrap-token-expired"></a>

## 一次性初始化令牌已过期

重启 CPAMP，使用日志中新签发的令牌重新打开初始化页。

<a id="bootstrap-token-invalid"></a>

## 一次性初始化令牌无效

完整复制最新启动日志中的令牌，不要包含空格或日志前缀。服务重启后旧令牌会失效。

<a id="slim-cpa-source-invalid"></a>

## Slim 的 CPA 来源选项无效

刷新初始化页，只选择“下载最新版 CPA”或“使用已有 CPA”。

<a id="slim-cpa-provision-unavailable"></a>

## 当前部署不能选择 CPA 来源

只有 Slim 包可以在初始化阶段下载 CPA 或切换到已有 CPA。外部连接、安装器管理和已经集成 CPA 的部署不会提供这些操作。

<a id="slim-cpa-download-failed"></a>

## Slim 下载或启动 CPA 失败

检查主机能否访问 GitHub Release、签名公钥是否配置、磁盘空间是否充足，以及当前平台和 libc 是否有匹配资产。修复后重试；已完成的 CPAMP 数据不会被删除。

<a id="slim-transition-recovery-pending"></a>

## Slim 切换正在等待恢复

CPAMP 已保存 CPA 连接，但 Runtime Control 尚未完成最终提交。确认 CPA 地址可访问、管理密钥仍然有效，并检查 Runtime Control 日志；修复后刷新初始化页或重启 CPAMP，系统会幂等重试，不需要重新部署。

<a id="panel-base-path-environment"></a>

## Base Path 由环境变量管理

移除或修改 `CPA_MANAGER_PANEL_BASE_PATH` 后重启。由环境指定时，UI 不允许动态覆盖。

<a id="panel-base-path-invalid"></a>

## Base Path 无效

使用 `/`、`/admin`、`/panel` 这类绝对路径。不要包含查询参数、片段、反斜杠、`.`/`..`，也不要占用 `/health`、`/setup`、`/v0`、`/v1` 等 API 路径。

<a id="runtime-update-not-managed"></a>

## 当前部署不支持托管更新

CPAMP 只为内置 CPA 的 Integrated/Full 部署管理 CPAMP 与 CPA 更新。外部 CPA 和一键安装器管理的分离部署继续使用原有升级方式。

<a id="runtime-update-check-failed"></a>

## 更新检查失败

检查运行时 manifest 地址、网络、系统时间和发布签名公钥。不要绕过签名或校验和验证。

<a id="runtime-update-start-failed"></a>

## 更新任务无法启动

确认没有另一个更新任务正在执行，数据目录可写且磁盘空间充足。查看系统页中的任务详情和服务日志。

<a id="runtime-operation-not-found"></a>

## 更新任务记录不存在

刷新系统页重新获取最新任务。旧任务可能已被轮转清理，或查询令牌来自另一个 CPAMP 实例。

<a id="runtime-control-unavailable"></a>

## CPAMP Runtime Control 不可用

确认服务通过 `runtime` 模式启动，内部控制端口没有被占用，并且 Manager 子进程继承了运行时控制地址与密钥。
