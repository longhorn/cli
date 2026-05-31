# syntax=docker/dockerfile:1.24.0@sha256:87999aa3d42bdc6bea60565083ee17e86d1f3339802f543c0d03998580f9cb89
FROM registry.suse.com/bci/golang:1.26@sha256:9b829b1026dac4281baa0715c4dd975735665d5ff843b0e1cd20e3fbbde95302 AS app_builder

WORKDIR /app

# Copy the build script and source code
COPY . /app

# Make the build script executable
RUN chmod +x /app/scripts/build

# Run the build script
RUN /app/scripts/build


FROM registry.suse.com/bci/golang:1.26@sha256:9b829b1026dac4281baa0715c4dd975735665d5ff843b0e1cd20e3fbbde95302 AS spdk_builder

ARG ARCH=amd64

RUN zypper -n ref && \
    zypper update -y

ENV SPDK_COMMIT_ID a6478cde7e0cff2fb09992868308a7387aa5202a
RUN cd /usr/src && \
    git clone https://github.com/longhorn/spdk.git /spdk && \
    cd /spdk && \
    git checkout ${SPDK_COMMIT_ID} && \
    rm -rf .git

FROM registry.suse.com/bci/bci-base:15.7@sha256:c2b0859ac7ceaf22c2d75a05c931dd7976dc0ac75e1a3a5f3c14380fcc3fb029 AS release

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
