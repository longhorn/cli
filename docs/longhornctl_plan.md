## longhornctl plan

Plan Longhorn data engine settings for a cluster

### Synopsis

Calculate data-engine-memory-size, data-engine-iobuf-large-pool-size and data-engine-iobuf-small-pool-size
of the Longhorn V2 data engine (SPDK). Each setting gets two values, both derived from the SPDK source, so they do not
depend on the hardware or on the workload:

  - At least: the smallest value with which spdk_tgt starts and every per-thread iobuf cache is filled (not below the
    minimum of the Longhorn settings). The bdev and accel modules keep a fixed cache on every thread, and the nvmf
    transport keeps half of each pool in per-poll-group caches. At this value the caches take the pool, so I/O stalls
    under load.
  - Highest: the value that covers the theoretical peak, with every queue of every engine full. A write command takes
    one buffer on the engine when it starts (132 KiB large, or 8 KiB small for a short command) and keeps it until all
    replicas have acknowledged it, and a queue holds at most 127 commands, so the engines of a node can hold at most
    F = engines x nr-io-queues x 127 buffers. A poll group serves requests from its own cache first and then from the
    shared ring, and cannot use the caches of the other poll groups, so a pool of P buffers gives the busiest group
    U(P) = P - thread caches - (poll groups - 1) x per-group cache. The highest pool is the smallest multiple of 1024
    with U(P) >= F plus the 31 buffers by which each other poll group cache can exceed its size. With it the engines can
    never take every buffer, so the replicas always get one and every write can finish, whatever the workload.
  - The memory size is the hugepage heap plus a headroom, rounded up to 128 MiB. The heap is computed from the SPDK source:
    the object pools and rings (sizes of the structures and the DPDK mempool layout), the two iobuf pools, and the DMA
    memory of the NVMe/TCP queue pairs of the front end and of the back end (in-capsule buffers, PDUs, requests). Memory
    that spdk_tgt takes from malloc, such as the cluster arrays of the blobs, is not in the hugepages, so the size and
    the snapshots of the volumes do not change the result.

Between the two values the result depends on the workload, which the command does not know. Use the highest values when
the memory allows. The highest values grow with nr-io-queues, so the output also lists them for other numbers of queues
per volume (default-nvme-tcp-nr-io-queues).

The estimate is for one node, the one with the most engines and replicas: give the number of engines on it (the volumes
attached to it) and the number of replicas on it (of any volume). Count the Engine and Replica resources by spec.nodeID,
for example with kubectl get engines.longhorn.io,replicas.longhorn.io -n longhorn-system -o wide. The output lists every
step with the numbers filled in.

Only one NVMe/TCP transport may exist in spdk_tgt. Creating two concurrently halves the cache of each and can crash
the instance manager at any pool size.

```
longhornctl plan [flags]
```

### Examples

```
  # A node with 17 engines and 50 replicas, 8 SPDK cores
  longhornctl plan --data-engine v2 --cpu-cores 8 --number-of-engines 17 --number-of-replicas 50

  # The same, with the data-engine-cpu-mask of the setting and JSON output
  longhornctl plan --data-engine v2 --cpu-mask 0xFF --number-of-engines 17 --number-of-replicas 50 -o json

  # With 2 NVMe I/O queues per volume instead of the default 4
  longhornctl plan --data-engine v2 --cpu-cores 8 --number-of-engines 17 --number-of-replicas 50 --nr-io-queues 2
```

### Options

```
      --cpu-cores int            Number of SPDK cores (bits of data-engine-cpu-mask). Use either this or --cpu-mask
      --cpu-mask string          Value of data-engine-cpu-mask, for example 0xFF. Use either this or --cpu-cores
      --data-engine string       Data engine to calculate for (v2)
  -h, --help                     help for plan
      --host-cpus int            CPUs of a node, used only when --nr-io-queues is 0
      --nr-io-queues int         Value of default-nvme-tcp-nr-io-queues. 0 is the kernel default, one queue for each host CPU, and needs --host-cpus (default 4)
      --number-of-engines int    Number of engines on a node, the node with the most: the volumes attached to it
      --number-of-replicas int   Number of replicas on a node, the node with the most, of any volume
  -o, --output string            Output format (text, json, yaml) (default "text")
      --ublk-cores int           Cores of the ublk target, 0 when the ublk frontend is not used
```

### Options inherited from parent commands

```
      --image string               Image containing longhornctl-local (default "longhornio/longhorn-cli:v1.13.0-dev")
      --image-pull-secret string   Secret with registry credentials for pulling images
      --image-registry string      Registry to apply to all images (CLI, engine, pause, BCI, etc.), replacing any registry already specified in those images.
      --kubeconfig string          Kubernetes config (kubeconfig) path
  -l, --log-level string           log level (trace, debug, info, warn, error, fatal, panic) (default "info")
      --namespace string           The namespace to run DaemonSet pods. (default "longhorn-system")
      --node-selector string       Comma-separated list of key=value pairs to match against node labels, selecting the nodes the DaemonSet will run on (e.g. env=prod,zone=us-west).
      --tolerations string         Semicolon-separated list of tolerations for DaemonSet pods (e.g. key=value:NoSchedule;:NoExecute).
```

### SEE ALSO

* [longhornctl](longhornctl.md)	 - Command-line interface for Longhorn.

###### Auto generated by spf13/cobra on 6-Jul-2026
