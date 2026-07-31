# Reverse Proxy

The new CPAMP Gateway handles the panel, management APIs, and model APIs through one entry. A reverse proxy no longer needs separate rules for `/management.html`, `/v0`, and `/v1`; forward all HTTP traffic to CPAMP `18137`.

```text
Internet / LAN
  -> HTTPS reverse proxy
      -> CPAMP :18137
          -> CPAMP Manager
          -> bundled or configured CPA
```

`18137`, `8137`, and `18317` have identical capabilities. Use `18137` for new configuration and keep older ports only during migration.

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

Inside a Docker network, use:

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

## Dynamic Base Path

Default management entry:

```text
https://cpamp.example.com/management.html
```

“System → Runtime & Updates” can change it dynamically to:

```text
https://cpamp.example.com/admin
https://cpamp.example.com/panel
https://cpamp.example.com/
```

With the `location /` or whole-site `reverse_proxy` examples above, the proxy normally needs no change. If your proxy allowlists individual paths, add the new Base Path before switching. Otherwise the browser redirects to a path the proxy rejects. The old entry returns 404 immediately.

The environment can lock the path:

```text
CPA_MANAGER_PANEL_BASE_PATH=/admin
```

The UI is read-only when the environment owns the value. A hidden path is not authentication or security control; continue using a strong CPAMP Admin Key, HTTPS, and appropriate IP, SSO, or zero-trust policy.

## Model APIs On The Same Domain

Every Gateway port can handle:

```text
/v1/*
/v1beta/*
/backend-api/*
/models
/v0/management/*
/usage-service/*
OAuth callbacks
```

API clients can therefore use:

```text
https://cpamp.example.com
```

CPAMP routes model and CPA paths to bundled or external CPA, while CPAMP management and analytics paths go to Manager Server.

## RESP Limitation

A normal HTTP reverse proxy cannot carry RESP Pub/Sub or RESP pop. In Integrated Full, CPAMP runtime connects to CPA internally, so `8317` does not need to be public.

Any RESP consumer outside CPAMP must connect directly to the CPA API port or internal address, not the Nginx/Caddy HTTP entry.

## Public Security

- Use HTTPS and disable plaintext public access.
- The CPAMP Admin Key must be at least 16 characters and satisfy the three-class policy.
- Never place the bootstrap token, CPAMP Admin Key, or CPA Management Key in URLs, access logs, or screenshots.
- Add firewall rules, IP allowlists, mTLS, SSO, or a zero-trust gateway when appropriate.
- Base Path only reduces discoverability and never replaces authentication.
- Anonymous `/usage-service/info` does not return the custom Base Path; the installer recovers it only from local runtime state.

## Verification

```bash
curl https://cpamp.example.com/health
curl https://cpamp.example.com/usage-service/info
curl https://cpamp.example.com/v1/models
```

After initialization:

```bash
curl -H "Authorization: Bearer <CPAMP_ADMIN_KEY>" \
  https://cpamp.example.com/status
```

After changing Base Path, verify that the new entry succeeds and the old entry returns 404.
