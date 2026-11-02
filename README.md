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

### V2-only prerequisites

The default `--data-engine-mode=v1-capable` retains iSCSI requirements. An explicitly selected `v2-only` mode omits iSCSI packages, modules and service operations, and requires SPDK checks and setup even without `--enable-spdk`.

Use matching CLI binaries and a CLI image containing this capability. The remote command passes an explicit flag to the local executable so an older image fails instead of silently running V1 checks.

```bash
longhornctl install preflight --data-engine-mode=v2-only --hugepage-size=2048 --allow-pci=none
longhornctl check preflight --data-engine-mode=v2-only --hugepage-size=2048
```

Installation success is prerequisite setup, followed by a separate host qualification check. A required reboot remains unfinished installation; reboot and rerun installation before checking. Restart kubelet when necessary for its Node capacity to reflect the hugepage allocation. V2 check failures return a nonzero exit status after printing the complete report.

V2 installation supports the known package-manager distributions. COS, Talos, unknown operating systems, unqualified NUMA layouts and host-provided SPDK startup hooks are rejected; configure immutable hosts through their own declarative prerequisites. The supported setup profile uses 2MiB pages on one existing NUMA node (node 0 by default). `HUGENODE` accepts one nonnegative integer. The requested memory, page size, PCI selection and driver must agree with any supplied `--spdk-options`; unknown, duplicate or conflicting options are rejected. Setup uses a bounded, cleared environment and verifies the actual allocation afterward.

An empty PCI selection is fenced as `none`. Explicit device selection requires unique full PCI addresses; do not select devices still in use. Installer `--driver-override` accepts `vfio-pci` or `uio_pci_generic`; checker `--userspace-driver` uses the kernel module names `vfio_pci` or `uio_pci_generic`.

V2 remote runs use separate resource names and collect complete reports from the identified DaemonSet and completed container instances. Cleanup deletes only objects created by that invocation, with UID preconditions. This mode neither converts existing volumes nor enables the manager's separate V2-only installation capability. Retire V1 data before removing its host prerequisites.

