# Reverse proxy

Set `BASE_URL` to the public URL and `SECURE_COOKIES=true` behind HTTPS.

Caddy:

```
travel.example.com {
	reverse_proxy 127.0.0.1:8080
}
```

nginx:

```
location / {
    proxy_pass http://127.0.0.1:8080;
    proxy_set_header Host $host;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
    client_max_body_size 60m;
}
```

Raise the body size limit to at least `MAX_UPLOAD_BYTES` for document uploads. For `AUTH_MODE=proxy`, have the proxy set `Remote-User` / `Remote-Email` after its own login.
