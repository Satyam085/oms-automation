# syntax=docker/dockerfile:1.7

# Build stage — uses BuildKit's TARGETOS/TARGETARCH so the same Dockerfile
# produces correct binaries on amd64 dev machines AND arm64 servers.
FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS build

ARG TARGETOS
ARG TARGETARCH

WORKDIR /src

# Cache module downloads. (No go.sum yet because no external deps — this
# still works; Go will write go.sum during build if/when deps appear.)
COPY go.mod ./
RUN go mod download

COPY . .

RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w" -o /out/oms-automation .

# Runtime stage — distroless static is ~2MB, has CA certs, no shell.
# tzdata is embedded in the binary via `time/tzdata` import, so we don't
# need a tzdata layer.
FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app
COPY --from=build /out/oms-automation /app/oms-automation

ENV RUN_MODE=server
ENV PORT=8080
EXPOSE 8080

USER nonroot:nonroot
ENTRYPOINT ["/app/oms-automation"]
