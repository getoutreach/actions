# syntax=docker/dockerfile:1.0-experimental

FROM gcr.io/outreach-docker/golang:1.23.4 as builder

ARG ACTION

ENV GOCACHE "/go-build-cache"
ENV GOPRIVATE github.com/getoutreach/*
ENV CGO_ENABLED 0

WORKDIR /src

# Copy our source code, go module information, and the files necessary
# to run make comands into the builder directory.
COPY actions/${ACTION}/ ./cmd/action/
COPY go.mod go.sum Makefile stencil.lock ./
COPY scripts/ ./scripts/
COPY pkg/ ./pkg/

# Cache dependencies across builds
RUN --mount=type=ssh --mount=type=cache,target=/go/pkg go mod download

# Build our application, caching the go build cache, but also using
# the dependency cache from earlier.
RUN --mount=type=ssh --mount=type=cache,target=/go/pkg --mount=type=cache,target=/go-build-cache \
    mkdir -p bin; \
    go build -v -o /src/bin/ ./cmd/...

FROM gcr.io/outreach-docker/alpine:3.18
ENTRYPOINT ["/usr/local/bin/action"]
LABEL "io.outreach.reporting_team"="fnd-dt"
LABEL "io.outreach.repo"="actions"

# Add timezone information.
COPY --from=builder /usr/local/go/lib/time/zoneinfo.zip /zoneinfo.zip
ENV ZONEINFO=/zoneinfo.zip

COPY --from=builder /src/bin/action /usr/local/bin/action
USER systemuser
