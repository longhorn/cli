# syntax=docker/dockerfile:1.27.0@sha256:bde3983e9c939224420ddaf6b784cc30e09b035a4dea01f581230c50809f372e
FROM golangci/golangci-lint:v2.13.2@sha256:ba07dffad130794ae79ebaa0056809d18c0168f3f846480ffd3eb6c04578b83d AS golangci-lint

FROM registry.suse.com/bci/golang:1.26@sha256:f6f04957fbc16399c76b1e8266cc7d0f3b4a5434b824fb251d7e3ae559d6a910 AS base

ARG TARGETARCH
ARG http_proxy
ARG https_proxy

ENV ARCH=${TARGETARCH}
ENV GOFLAGS=-mod=vendor

# Install packages
RUN zypper update -y && \
    zypper -n install curl awk && \
    rm -rf /var/cache/zypp/*

# Copy golangci-lint binary from official image
COPY --from=golangci-lint /usr/bin/golangci-lint /usr/local/bin/golangci-lint

WORKDIR /go/src/github.com/longhorn/cli
COPY . .

FROM base AS build
RUN ./scripts/build

FROM base AS validate
RUN ./scripts/validate && touch /validate.done

FROM base AS test
RUN ./scripts/test

FROM scratch AS build-artifacts
COPY --from=build /go/src/github.com/longhorn/cli/bin/ /bin/

FROM scratch AS ci-artifacts
COPY --from=build /go/src/github.com/longhorn/cli/bin/ /bin/
COPY --from=validate /validate.done /validate.done
COPY --from=test /go/src/github.com/longhorn/cli/coverage.out /coverage.out
