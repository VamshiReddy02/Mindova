# syntax=docker/dockerfile:1

# ---- Builder ---------------------------------------------------------
#
# Mindova is a Go workspace (go.work), not a single module: document-service,
# packages/kernel, and app/example-service each have their own go.mod, and
# document-service imports packages/kernel by local filesystem resolution
# (go.work "use" directives), not by fetching a versioned module from a
# proxy. That means the builder needs the WHOLE workspace physically
# present — copying only services/document-service would leave `go build`
# unable to resolve github.com/vamshireddy02/mindova/packages/kernel/...
# imports at all.
#
# Base image version matches go.work's `go 1.26.3` directive. If your
# installed toolchain requires an exact patch, pin golang:1.26.3-alpine
# instead of the floating 1.26-alpine tag.
FROM golang:1.26-alpine AS builder

WORKDIR /src

# Copy the workspace file(s) and every module go.work references first,
# so `go build` can resolve dependencies before the rest of the source
# changes on later, more frequent rebuilds. go.work.sum may not exist
# yet in every checkout; the glob is safe if it matches nothing.
COPY go.work go.work.sum* ./
COPY packages/kernel ./packages/kernel
COPY services/document-service ./services/document-service
COPY app/example-service ./app/example-service

WORKDIR /src/services/document-service

# CGO_ENABLED=0 produces a fully static binary — no libc dependency,
# which is what makes an empty/distroless runtime stage possible below.
# -trimpath removes local build-machine file paths from the binary;
# -ldflags="-s -w" strips debug symbols, shrinking the binary further
# (fine for production; skip both flags if you need `go tool pprof` or
# a debugger to work against this exact binary later).
#
# Build target is ./cmd, not ./cmd/document-service: main.go lives
# directly in services/document-service/cmd/, matching `go run
# ./services/document-service/cmd/` — cmd/run-worker is the only actual
# subdirectory under cmd/.
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags="-s -w" \
    -o /out/document-service \
    ./cmd

# ---- Runtime -----------------------------------------------------------
#
# distroless/static: no shell, no package manager, no OS userland at
# all beyond what a static Go binary needs (CA certs, /etc/passwd for
# the nonroot user, tzdata). This is deliberately more restrictive than
# Alpine — there is no `docker exec -it ... sh` into this container,
# because there's no shell to exec into. That's the point: a smaller
# attack surface for a production image. If you need a shell for local
# debugging, swap this stage's FROM line for alpine:3.20 and add
# `RUN apk add --no-cache ca-certificates`.
FROM gcr.io/distroless/static-debian12:nonroot AS runtime

WORKDIR /app
COPY --from=builder /out/document-service /app/document-service

EXPOSE 8080

USER nonroot:nonroot

ENTRYPOINT ["/app/document-service"]
