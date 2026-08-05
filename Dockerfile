# syntax=docker/dockerfile:1

FROM --platform=$BUILDPLATFORM golang:1.25.12-alpine AS builder

ARG TARGETOS
ARG TARGETARCH

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 \
    GOOS=$TARGETOS \
    GOARCH=$TARGETARCH \
    go build \
    -trimpath \
    -ldflags="-s -w" \
    -o /out/aquawheel-api \
    ./cmd/api/main.go

RUN CGO_ENABLED=0 \
    GOOS=$TARGETOS \
    GOARCH=$TARGETARCH \
    go build \
    -trimpath \
    -ldflags="-s -w" \
    -o /out/ecommerce-migrate \
    ./cmd/migrate/main.go

RUN CGO_ENABLED=0 \
    GOOS=$TARGETOS \
    GOARCH=$TARGETARCH \
    go build \
    -trimpath \
    -ldflags="-s -w" \
    -o /out/ecommerce-cli \
    ./cmd/cli/main.go


# Сертифікати генеруються на архітектурі GitLab Runner,
# бо сам файл сертифікатів не залежить від CPU.
FROM --platform=$BUILDPLATFORM alpine:3.22 AS certificates

RUN apk add --no-cache ca-certificates


FROM alpine:3.22 AS runner

WORKDIR /app

COPY --from=certificates \
    /etc/ssl/certs/ca-certificates.crt \
    /etc/ssl/certs/ca-certificates.crt

FROM runner AS api

COPY --from=builder --chown=10001:10001 \
    /out/aquawheel-api \
    ./aquawheel-api

ENV SSL_CERT_FILE=/etc/ssl/certs/ca-certificates.crt

USER 10001:10001

EXPOSE 8080

CMD ["./aquawheel-api"]

FROM runner AS migrate

COPY --from=builder --chown=10001:10001 \
    /out/ecommerce-migrate \
    ./ecommerce-migrate
COPY --from=builder --chown=10001:10001 \
    /app/migrations \
    ./migrations

CMD ["./ecommerce-migrate", "up"]

FROM runner AS cli

COPY --from=builder --chown=10001:10001 \
    /out/ecommerce-cli \
    ./ecommerce-cli

ENTRYPOINT ["./ecommerce-cli"]
