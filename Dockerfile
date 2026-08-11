# syntax=docker/dockerfile:1.7

FROM --platform=$BUILDPLATFORM golang:1.25.12-alpine AS builder

ARG TARGETOS
ARG TARGETARCH

WORKDIR /src

# Keep dependency and compiler caches outside the image layers. BuildKit reuses
# them across builds while the final image remains free of source and tooling.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . ./

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w" -o /out/ecommerce-api ./cmd/api

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w" -o /out/ecommerce-migrate ./cmd/migrate

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w" -o /out/ecommerce-cli ./cmd/cli

# The distroless static image does not need a shell or package manager. Copy
# the CA bundle explicitly because payment, SMTP, and OTLP adapters use TLS.
FROM alpine:3.22 AS certificates
RUN apk add --no-cache ca-certificates

FROM gcr.io/distroless/static-debian12:nonroot AS runtime
WORKDIR /app

COPY --from=certificates /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
ENV SSL_CERT_FILE=/etc/ssl/certs/ca-certificates.crt

FROM runtime AS migrate
COPY --from=builder --chown=nonroot:nonroot /out/ecommerce-migrate /app/ecommerce-migrate
COPY --from=builder --chown=nonroot:nonroot /src/migrations /app/migrations
USER nonroot:nonroot
ENTRYPOINT ["/app/ecommerce-migrate"]
CMD ["up"]

FROM runtime AS cli
COPY --from=builder --chown=nonroot:nonroot /out/ecommerce-cli /app/ecommerce-cli
USER nonroot:nonroot
ENTRYPOINT ["/app/ecommerce-cli"]

# API is intentionally the final/default target: `docker build .` produces
# the production runtime image rather than a migration or maintenance tool.
FROM runtime AS api
COPY --from=builder --chown=nonroot:nonroot /out/ecommerce-api /app/ecommerce-api
USER nonroot:nonroot
EXPOSE 8080 9090
ENTRYPOINT ["/app/ecommerce-api"]
