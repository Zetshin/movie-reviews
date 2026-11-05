# syntax=docker/dockerfile:1

# ---- build ---------------------------------------------------------------
FROM golang:1.27.3-alpine AS build
WORKDIR /src

# Download modules first so this layer is cached between code changes.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY cmd ./cmd
COPY internal ./internal

ARG VERSION=dev
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /out/web ./cmd/web

# Uploads live in a volume; create the folders here so they are owned by
# the non-root runtime user (65532) when Docker initialises the volume.
RUN mkdir -p /out/uploads/avatars /out/uploads/movies /out/uploads/persons

# ---- runtime -------------------------------------------------------------
# Distroless: no shell or package manager, runs as an unprivileged user.
FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app

COPY --from=build /out/web /app/web
COPY ui ./ui
COPY --from=build --chown=nonroot:nonroot /out/uploads ./ui/static/uploads

ENV ADDR=:4000 \
    TLS_ENABLED=false \
    SECURE_COOKIE=true \
    TRUST_PROXY=true \
    DB_MIGRATE=true

EXPOSE 4000
USER nonroot:nonroot

HEALTHCHECK --interval=30s --timeout=5s --start-period=60s --retries=3 \
    CMD ["/app/web", "-healthcheck"]

ENTRYPOINT ["/app/web"]
