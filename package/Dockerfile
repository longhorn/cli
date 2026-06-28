# syntax=docker/dockerfile:1.17.1@sha256:38387523653efa0039f8e1c89bb74a30504e76ee9f565e25c9a09841f9427b05
FROM registry.suse.com/bci/golang:1.25@sha256:1cb7fe65189006b99b3246d8536b75f30eef41e50586df415eaf143ea204dcef AS app_builder

WORKDIR /app

# Copy the build script and source code
COPY . /app

# Make the build script executable
RUN chmod +x /app/dapper/build

# Run the build script
RUN /app/dapper/build


FROM registry.suse.com/bci/golang:1.25@sha256:1cb7fe65189006b99b3246d8536b75f30eef41e50586df415eaf143ea204dcef AS spdk_builder

ARG ARCH=amd64

RUN zypper -n ref && \
    zypper update -y

ENV SPDK_COMMIT_ID a6478cde7e0cff2fb09992868308a7387aa5202a
RUN cd /usr/src && \
    git clone https://github.com/longhorn/spdk.git /spdk && \
    cd /spdk && \
    git checkout ${SPDK_COMMIT_ID} && \
    rm -rf .git

FROM registry.suse.com/bci/bci-base:15.7@sha256:4eb60eddf5a5184dc53ce0983003b865a8a164bc0edf18103a5225fb37bb3813 AS release

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
