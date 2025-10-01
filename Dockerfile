FROM golang:1.25-alpine AS builder

WORKDIR /src

# Install git (for go modules) and CA certs
RUN apk add --no-cache git ca-certificates && update-ca-certificates

# Pre-cache go modules
COPY go.mod ./
RUN go mod download

# Copy source
COPY . .

# Build static binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags='-s -w' -o /out/giscus-wrapper ./

# -------- Runtime stage --------
FROM gcr.io/distroless/static:nonroot

WORKDIR /
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder /out/giscus-wrapper /giscus-wrapper

USER nonroot:nonroot

ENV HOST=0.0.0.0
ENV PORT=8080

EXPOSE 8080

ENTRYPOINT ["/giscus-wrapper"]

