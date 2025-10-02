## giscus-proxy

Minimal proxy for the public giscus widget so you can embed it from your own origin.

<a href="https://railway.com/deploy/OhvyYk"><img src="https://railway.app/button.svg" alt="Deploy on Railway" height="32" /></a>
<a href="https://render.com/deploy?repo=https://github.com/cdlus/giscus-proxy"><img src="https://render.com/images/deploy-to-render-button.svg" alt="Deploy to Render" height="32" /></a>
<a href="https://vercel.com/new/clone?repository-url=https://github.com/cdlus/giscus-proxy"><img src="https://vercel.com/button" alt="Deploy with Vercel" height="32" /></a>
<a href="https://app.netlify.com/start/deploy?repository=https://github.com/cdlus/giscus-proxy"><img src="https://www.netlify.com/img/deploy/button.svg" alt="Deploy to Netlify" height="32" /></a>
<a href="https://heroku.com/deploy/?template=https://github.com/cdlus/giscus-proxy"><img src="https://www.herokucdn.com/deploy/button.svg" alt="Deploy to Heroku" height="32" /></a>
<a href="https://app.koyeb.com/deploy?type=git&repository=github.com/cdlus/giscus-proxy&branch=main&name=giscus-proxy"><img src="https://www.koyeb.com/static/images/deploy/button.svg" alt="Deploy to Koyeb" height="32" /></a>

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
go run ./cmd/giscus-proxy
# or with custom port
PORT=9000 go run ./cmd/giscus-proxy
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

## Deploy to Vercel

The repository ships with a serverless handler under `api/index.go` and a
`vercel.json` rewrite that forwards every request to it. Deploying through the
"Deploy with Vercel" button or via `vercel deploy` builds a Go Serverless
Function, so no Docker support is required. Once deployed, the instance exposes
the same routes described above.

You can set `HOST`, `PORT`, or `ADDR` environment variables if you want to run
the binary locally with `vercel dev`, but they are not required for production
deployments on Vercel.

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