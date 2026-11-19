# Longhorn Commandline Interface (longhornctl)

This repository contains the source code for `longhornctl`, a CLI (command-line interface) designed to simplify Longhorn manual operations.

## What Can You Do With `longhornctl`?

- Install and verify prelight requirements.
- Execute one-time Longhorn operations.
- Gain insight into your Longhorn system.
- Diagnose the health of your Longhorn system.

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
1. Validate your changes before opening a pull request
    ```bash
    make validate
    ```
    > **Note:** Besides `go vet`, `golangci-lint`, and `go fmt`, this runs the [ASCII check](https://github.com/longhorn/longhorn/blob/master/CONTRIBUTING.md#ascii-only-content) on the files and commit messages changed since your branch base. The changes are computed on the host before the containerized validation runs, so this also works in a git worktree.

    The branch base is the merge base of `HEAD` and `UPSTREAM_BASE`. By default, `UPSTREAM_BASE` is the default branch of whichever remote points at `github.com/longhorn/cli`, so a fork named `origin` next to an `upstream` remote works without setup. Set `UPSTREAM_BASE` to any commit or ref when the default does not match the branch your pull request targets:
    ```bash
    # A backport targeting a release branch
    make validate UPSTREAM_BASE=upstream/v1.10.x

    # No github.com/longhorn/cli remote configured, e.g. only a fork as origin
    git fetch https://github.com/longhorn/cli.git master
    make validate UPSTREAM_BASE=FETCH_HEAD

    # A branch stacked on another feature branch: check only your own commits
    make validate UPSTREAM_BASE=my-feature-base
    ```
    Fetch the remote first so the ref is current. If no base can be found, for example in a shallow clone, `make validate` prints a message and skips the ASCII check.

