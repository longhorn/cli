# syntax=docker/dockerfile:1.28.0@sha256:bb22d9815c728170f72750f4e5b0d672e06176142e1d602c7e66c050100b7e5b
FROM registry.suse.com/bci/golang:1.27@sha256:634cd6f1e373bcde01a1f8df6d1c3a9784142353a0842b5371d6385b4e9d2abb AS app_builder

WORKDIR /app

# Copy the build script and source code
COPY . /app

# Make the build script executable
RUN chmod +x /app/scripts/build

# Run the build script
RUN /app/scripts/build


FROM registry.suse.com/bci/golang:1.27@sha256:634cd6f1e373bcde01a1f8df6d1c3a9784142353a0842b5371d6385b4e9d2abb AS spdk_builder

ARG ARCH=amd64

RUN zypper -n ref && \
    zypper update -y

ENV SPDK_COMMIT_ID a6478cde7e0cff2fb09992868308a7387aa5202a
RUN cd /usr/src && \
    git clone https://github.com/longhorn/spdk.git /spdk && \
    cd /spdk && \
    git checkout ${SPDK_COMMIT_ID} && \
    rm -rf .git

FROM registry.suse.com/bci/bci-base:15.7@sha256:927140d8ede708eed63e8ce3c789801288403c23ac53a40ced59164abc938103 AS release

ARG TARGETPLATFORM
RUN if [ "$TARGETPLATFORM" != "linux/amd64" ] && [ "$TARGETPLATFORM" != "linux/arm64" ]; then \
    echo "Error: Unsupported TARGETPLATFORM: $TARGETPLATFORM" && \
    exit 1; \
    fi

ENV ARCH ${TARGETPLATFORM#linux/}

RUN zypper -n ref && \
    zypper update -y

RUN zypper -n install jq && \
    rm -rf /var/cache/zypp/*

COPY --from=app_builder /app/bin/longhornctl-linux-${ARCH} /usr/local/bin/longhornctl
COPY --from=app_builder /app/bin/longhornctl-local-linux-${ARCH} /usr/local/bin/longhornctl-local

COPY --from=spdk_builder \
   /spdk \
   /spdk

CMD ["longhornctl"]
