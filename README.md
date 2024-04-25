# Longhorn Commandline Interface (longhornctl)

This repository contains the source code for `longhornctl`, a CLI (command-line interface) designed to simplify Longhorn manual operations.

## What Can You Do With `longhornctl`?

- Install and verify prelight requirements.
- Execute one-time Longhorn operations.
- Gain inside into your Longhorn system.

## Install `longhornctl`

### Run From Container Image

`To be updated`

### Using curl

`To be updated`

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
1. After the build process completes, find the `longhornctl` binary in the `./bin` directory.

## Getting Started

To begin, run `longhornctl --help` to access a list of available commands and options.

### Command Reference

`To be updated`