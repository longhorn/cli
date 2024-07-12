# syntax=docker/dockerfile:1.8.1

FROM registry.suse.com/bci/bci-base:15.6

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

COPY bin/longhornctl-linux-${ARCH} /usr/local/bin/longhornctl
COPY bin/longhornctl-local-linux-${ARCH} /usr/local/bin/longhornctl-local

COPY spdk /spdk

CMD ["longhornctl"]
