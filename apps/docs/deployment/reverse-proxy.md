# 反向代理

新版 CPAMP Gateway 已统一处理面板、管理接口和模型 API。反向代理不再需要按 `/management.html`、`/v0`、`/v1` 分别转发给 CPAMP 和 CPA；把 HTTP 流量整体转发到 CPAMP `18137` 即可。

```text
Internet / LAN
  -> HTTPS reverse proxy
      -> CPAMP :18137
          -> CPAMP Manager
          -> 内置或已配置的 CPA
```

`18137`、`8137`、`18317` 能力一致。新配置使用 `18137`，旧端口只用于兼容迁移。

## Nginx

```nginx
upstream cpamp_gateway {
    server 127.0.0.1:18137;
    keepalive 32;
}

server {
    listen 443 ssl http2;
    server_name cpamp.example.com;

    ssl_certificate     /etc/nginx/ssl/cert.pem;
    ssl_certificate_key /etc/nginx/ssl/key.pem;

    client_max_body_size 64m;

    location / {
        proxy_pass http://cpamp_gateway;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_read_timeout 600s;
        proxy_send_timeout 600s;
    }
}
```

Docker network 中把 upstream 改为：

```nginx
upstream cpamp_gateway {
    server cpa-manager-plus:18137;
}
```

## Caddy

```caddyfile
cpamp.example.com {
    reverse_proxy 127.0.0.1:18137
}
```

## 动态 Base Path

默认管理入口：

```text
https://cpamp.example.com/management.html
```

可以在“系统 → 运行时与更新”动态改为：

```text
https://cpamp.example.com/admin
https://cpamp.example.com/panel
https://cpamp.example.com/
```

如果 Nginx/Caddy 使用上面的 `location /` 或整站 `reverse_proxy`，通常不需要修改反向代理。若你只允许特定路径，必须在切换前加入新 Base Path，否则保存后浏览器会跳到代理未放行的地址。旧入口会立即返回 404。

也可以由环境变量锁定：

```text
CPA_MANAGER_PANEL_BASE_PATH=/admin
```

环境变量配置时 UI 只读。隐藏路径不是认证或安全控制，仍需强 CPAMP 管理密钥、HTTPS 和必要的 IP/SSO/零信任访问策略。

## 同域名模型 API

所有 Gateway 端口都可以直接处理：

```text
/v1/*
/v1beta/*
/backend-api/*
/models
/v0/management/*
/usage-service/*
OAuth callbacks
```

因此 API 客户端可以把 base URL 指向：

```text
https://cpamp.example.com
```

CPAMP 会把模型和 CPA 路径转发到内置或外部 CPA，把 CPAMP 管理/分析路径交给 Manager Server。

## RESP 限制

普通 HTTP 反向代理不能承载 RESP Pub/Sub 或 RESP pop。Integrated Full 中 CPAMP runtime 会在内部直接连接 CPA，不需要公开 `8317`。

如果仍有 CPAMP 之外的 RESP 消费者，必须让它直连 CPA API 端口或内网地址，不要指向 Nginx/Caddy HTTP 入口。

## 公网安全

- 使用 HTTPS，禁用明文公网访问。
- CPAMP 管理密钥至少 16 位并满足三类字符策略。
- 不要把一次性 bootstrap token、CPAMP 管理密钥或 CPA Management Key 写进 URL、访问日志或截图。
- 可额外使用防火墙、IP allowlist、mTLS、SSO 或零信任网关。
- Base Path 只降低可发现性，不能替代认证。
- 匿名 `/usage-service/info` 不返回自定义 Base Path；安装器只从本机运行时状态恢复入口。

## 验证

```bash
curl https://cpamp.example.com/health
curl https://cpamp.example.com/usage-service/info
curl https://cpamp.example.com/v1/models
```

初始化后验证管理接口：

```bash
curl -H "Authorization: Bearer <CPAMP_ADMIN_KEY>" \
  https://cpamp.example.com/status
```

修改 Base Path 后，同时验证新入口成功、旧入口为 404。
