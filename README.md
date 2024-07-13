# Longhorn Commandline Interface (longhornctl)

This repository contains the source code for `longhornctl`, a CLI (command-line interface) designed to simplify Longhorn manual operations.

## What Can You Do With `longhornctl`?

- Install and verify prelight requirements.
- Execute one-time Longhorn operations.
- Gain inside into your Longhorn system.

## Usage

For detailed usage information and examples, please refer to the [documents](./docs/longhornctl.md) run `longhornctl --help`.

You can obtain `longhornctl` either through downloading a prebuilt binary or by building it from source.

### Prebuilt Binary

Download the latest release suitable for your operating system and machine architecture from the [GitHub release page](https://github.com/longhorn/cli/releases). Then, rename it to `/usr/local/bin/longhornctl`.

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

