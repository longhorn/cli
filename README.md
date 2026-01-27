# Longhorn Commandline Interface (longhornctl)

This repository contains the source code for `longhornctl`, a CLI (command-line interface) designed to simplify Longhorn manual operations.

## What Can You Do With `longhornctl`?

- Install and verify prelight requirements.
- Execute one-time Longhorn operations.
- Gain insight into your Longhorn system.

## Usage

For detailed usage information and examples, please refer to the [document](./docs/longhornctl.md) run `longhornctl --help`.

You can obtain `longhornctl` either through downloading a prebuilt binary or by building it from source.

### Prebuilt Binary

1. Remove any previous `longhornctl` installation.

    ```
    rm -rf /usr/local/bin/longhornctl
    ```

2. Download the command-line tool release suitable for your operating system and machine architecture from the [GitHub release page](https://github.com/longhorn/cli/releases).

    ```
    curl -L https://github.com/longhorn/cli/releases/download/${LonghornVersion}/longhornctl-${OS}-${ARCH} -o longhornctl
    chmod +x longhornctl
    mv ./longhornctl /usr/local/bin/longhornctl
    ```

3. Verify that you've installed

    ```
    longhornctl version
    ```

### Build From Source

1. Clone repository
    ```bash
    git clone https://github.com/longhorn/cli.git
    ```
1. Build the `longhornctl` binary
    ```bash
    cd cli
    make
    ```
    > **Note:** This process will generate two binaries:
    >   - `longhornctl`: A command-line interface for remote Longhorn operations, designed to be run outside the Kubernetes cluster. It executes `longhornctl-local` for operations within the cluster.
    >   - `longhornctl-local`: A command-line interface to be used within a DaemonSet pod inside the Kubernetes cluster, handling in-cluster and host operations.

