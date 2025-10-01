## giscus-proxy

Minimal proxy for the public giscus widget so you can embed it from your own origin.

[![Deploy on Railway](https://railway.app/button.svg)](https://railway.app/template/new?template=https://github.com/cdlus/giscus-proxy)
[![Deploy to Render](https://render.com/images/deploy-to-render-button.svg)](https://render.com/deploy?repo=https://github.com/cdlus/giscus-proxy)

### Endpoints
- `GET /widget` → `https://giscus.app/en/widget` (with optional replacements via `rep=`)
- `GET /en/widget` → `https://giscus.app/en/widget` (alias)
- All other paths are proxied unchanged to `https://giscus.app/<same-path>`

### Configure
- `HOST` (default `0.0.0.0`) and `PORT` (default `8080`)
- Or set `ADDR` (e.g. `:8080` or `127.0.0.1:8080`). `ADDR` beats `HOST`/`PORT`.

---

## Run locally
```bash
go run .
# or with custom port
PORT=9000 go run .
```

---

## Docker

Build and run:
```bash
docker build -t giscus-proxy:latest .
docker run --rm -p 8080:8080 -e PORT=8080 giscus-proxy:latest
```

Override port:
```bash
docker run --rm -p 9000:9000 -e PORT=9000 giscus-proxy:latest
```

---

## Deploy to Railway

1) Create a new service from this repo.
2) Railway detects the Dockerfile automatically.
3) Set variables:
   - `PORT` = `8080` (Railway injects `PORT`; our app reads it)
   - Optional: `HOST` = `0.0.0.0`
4) Expose the service; Railway gives you a public URL.

Health check path: `/widget`.

---

## Deploy to a generic VPS

### Option A: Docker on VPS
```bash
git clone <this-repo>
cd giscus-proxy
docker build -t giscus-proxy:latest .
docker run -d --name giscus-proxy \
  -p 8080:8080 \
  -e HOST=0.0.0.0 -e PORT=8080 \
  --restart unless-stopped \
  giscus-proxy:latest
```

Put a reverse proxy in front (Caddy / Nginx / Traefik) if you want HTTPS and a domain.