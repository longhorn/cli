# Longhorn Preflight

`longhorn-preflight` helps install, configure and check the prerequisites for Longhorn system.

## Install

### Deploy

Users can create `longhorn-preflight` DaemonSet for installing and configuring the prerequisites and the environment.

```
# kubectl -f deploy/install.yaml
```

### Tweak the Options

#### General Options
- `UPDATE_PACKAGE_LIST`: Update package list before install required packages.

#### SPDK Specific Options
- `ENABLE_SPDK`: Enable installation of required packages, modules and setup.
- `HUGEMEM`: Hugepage size in MiB for SPDK.
- `PCI_ALLOWED`: Whitespace separated list of PCI devices. By default, block all PCI devices use a non-valid address.
- `DRIVER_OVERRIDE`: Bind devices to the given user space driver.

## Check

### Deploy

Users can create `longhorn-preflight` DaemonSet for checking the prerequisites and the environment.

```
# kubectl -f deploy/check.yaml
```

### Tweak the Options

#### SPDK Specific Options
- `ENABLE_SPDK`: Enable installation of required packages, modules and setup.
- `HUGEMEM`: Hugepage size in MiB for SPDK.
- `UIO_DRIVER`: Userspace IO driver.

