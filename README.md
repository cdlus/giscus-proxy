## giscus-proxy

Minimal proxy for the public giscus widget so you can embed it from your own origin.

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

## Host on GitHub

1) Create a new GitHub repository and push this project.
2) CI will run on push (Go vet/build + Docker build).
3) Optional: enable GitHub Container Registry publishing. Example workflow snippet to push images:
```yaml
      - name: Login to GHCR
        uses: docker/login-action@v3
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}

      - name: Build and push
        uses: docker/build-push-action@v6
        with:
          context: .
          push: true
          tags: ghcr.io/${{ github.repository }}/giscus-proxy:latest
```

Then deploy that image to your VPS or Railway.

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

Put a reverse proxy in front (Caddy / Nginx) if you want HTTPS and a domain.

### Option B: systemd (bare metal)
Install Go 1.22+, then:
```bash
git clone <this-repo>
cd giscus-proxy
go build -ldflags='-s -w' -o /usr/local/bin/giscus-wrapper .
sudo tee /etc/systemd/system/giscus-wrapper.service >/dev/null <<'UNIT'
[Unit]
Description=giscus wrapper
After=network-online.target
Wants=network-online.target

[Service]
ExecStart=/usr/local/bin/giscus-wrapper
Environment=HOST=0.0.0.0
Environment=PORT=8080
Restart=always
RestartSec=3
User=www-data
Group=www-data

[Install]
WantedBy=multi-user.target
UNIT

sudo systemctl daemon-reload
sudo systemctl enable --now giscus-wrapper
```

---

## Usage notes
- `rep` is only honored on `/widget`. Format `LEFT=>RIGHT`; regex via `re:LEFT=>RIGHT`.
- A built-in swap removes the “powered by giscus” footer in the widget HTML.
- CORS is permissive (`*`) for embedding convenience.

