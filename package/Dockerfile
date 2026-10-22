# syntax=docker/dockerfile:1.23.0@sha256:2780b5c3bab67f1f76c781860de469442999ed1a0d7992a5efdf2cffc0e3d769
FROM registry.suse.com/bci/golang:1.26@sha256:106219ac24d7002a0c393fe4be9530429ba0edb63548f42852cc38a9a40ba544 AS app_builder

WORKDIR /app

# Copy the build script and source code
COPY . /app

# Make the build script executable
RUN chmod +x /app/scripts/build

# Run the build script
RUN /app/scripts/build


FROM registry.suse.com/bci/golang:1.26@sha256:106219ac24d7002a0c393fe4be9530429ba0edb63548f42852cc38a9a40ba544 AS spdk_builder

ARG ARCH=amd64

RUN zypper -n ref && \
    zypper update -y

ENV SPDK_COMMIT_ID a6478cde7e0cff2fb09992868308a7387aa5202a
RUN cd /usr/src && \
    git clone https://github.com/longhorn/spdk.git /spdk && \
    cd /spdk && \
    git checkout ${SPDK_COMMIT_ID} && \
    rm -rf .git

FROM registry.suse.com/bci/bci-base:15.7@sha256:9b8660b8c83ddb0154fa62fac5681484e6cde1e4ce507704e7f4b232e2b1f108 AS release

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
