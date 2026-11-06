# longhornctl plan

`longhornctl plan --data-engine v2` calculates three settings of the Longhorn V2 data engine (SPDK) for one node:

- `data-engine-iobuf-large-pool-size`
- `data-engine-iobuf-small-pool-size`
- `data-engine-memory-size`

Each setting gets two values:

| Value | Meaning |
|---|---|
| **At least** | The smallest value with which `spdk_tgt` starts and every per-thread iobuf cache is filled. The caches take the pool, so I/O stalls under load. |
| **Highest** | The value that covers the theoretical peak of commands holding a buffer, whatever the workload. The engines can never take every buffer, so the replicas always get one and every write can finish. |

Everything is derived from the SPDK source (longhorn/spdk `longhorn-v26.05`). Nothing is fitted or measured, and the
command needs no information about the workload. Measured heaps are used only in the unit tests.

The code is in [`dataengine/`](./dataengine):

| File | Content |
|---|---|
| `calculator.go` | Inputs, validation, cache-fit minimum, usable buffers, highest pools, steps and notes |
| `memory.go` | Hugepage heap, computed from structure sizes and the DPDK mempool and ring layout |
| `render.go` | Text, JSON and YAML output |
| `calculator_test.go` | Unit tests |

The command is in `cmd/remote/subcmd/plan.go`, and the generated help is in `docs/longhornctl_plan.md`.

## Usage

The estimate is for one node, the one with the most engines and replicas.

```bash
# 8 SPDK cores, 17 engines and 50 replicas on the node, 4 NVMe I/O queues per volume (the default)
longhornctl plan --data-engine v2 --cpu-cores 8 --number-of-engines 17 --number-of-replicas 50

# The cpu-mask of the setting, and JSON output
longhornctl plan --data-engine v2 --cpu-mask 0xFF --number-of-engines 17 --number-of-replicas 50 -o json

# 2 NVMe I/O queues per volume
longhornctl plan --data-engine v2 --cpu-cores 8 --number-of-engines 17 --number-of-replicas 50 --nr-io-queues 2
```

| Flag | Meaning |
|---|---|
| `--cpu-cores` or `--cpu-mask` | SPDK cores (bits of `data-engine-cpu-mask`) |
| `--number-of-engines` | Engines on the node, the volumes attached to it |
| `--number-of-replicas` | Replicas on the node, of any volume |
| `--nr-io-queues` | `default-nvme-tcp-nr-io-queues` (default 4). 0 is the kernel default, one queue per host CPU, and needs `--host-cpus` |
| `--host-cpus` | CPUs of a node, used only when `--nr-io-queues` is 0 |
| `--ublk-cores` | Cores of the ublk target, 0 when the ublk frontend is not used |
| `-o` | `text` (default), `json` or `yaml` |

Count the engines and replicas of a node by `spec.nodeID`, for example with
`kubectl get engines.longhorn.io,replicas.longhorn.io -n longhorn-system -o wide`.

The output lists the settings, the highest values for 1, 2, 4 and 8 queues per volume, the memory breakdown, and every
calculation step with its numbers and its basis (`SPDK source`, `input` or `design choice`).

For 8 cores, 17 engines and 50 replicas:

| | At least | Highest |
|---|---|---|
| large pool | 1024 | 16384 |
| small pool | 8192 | 19456 |
| memory size (MiB) | 1152 | 3328 |

## How the pools are calculated

Notation: `G` poll groups = cores - 1 (the first core is the admin core), `T` threads = cores, `V` engines, `R` replicas.

### At least: cache fit

The bdev and accel modules keep a fixed iobuf cache on every thread (128 small and 16 large buffers each; ublk adds 128 and
32 per ublk core). The nvmf transport keeps half of each pool in per-poll-group caches. `spdk_tgt` starts when every cache
can be filled, that is the smallest `P` with

    floor(floor(P / 2) / G) x G + 2 x cache x T + ublk <= P

The condition is not monotonic in `P`, so the command takes the last failing `P` plus one. The result is not below the
minimum of the Longhorn settings (1024 large, 8192 small).

### Highest: the theoretical peak

A write takes one buffer when it starts and keeps it until all replicas have acknowledged it (`lib/nvmf/tcp.c`,
`lib/nvmf/transport.c`). The buffer is a 132 KiB large buffer, or an 8 KiB small buffer when the command is at most that
long. A queue holds at most 127 commands, so the engines of a node can hold at most

    F = V x nr-io-queues x 127

buffers, whatever the workload. Each pool must cover `F` alone, because a command can use either size.

A poll group takes from its own cache first and then from the shared ring, and a freed buffer goes back to the ring only
when the cache is 32 over its size. The busiest poll group therefore cannot use the caches of the others:

    U(P) = P - 2 x cache x T - ublk - (G - 1) x floor(floor(P / 2) / G)

The highest pool is the smallest multiple of 1024 with

    U(P) >= F + 31 x (G - 1)

where 31 is how far a poll group cache can exceed its size. This is a sufficient condition from the source. It was never
reproduced as a failure, and the command does not claim that a smaller pool fails.

The highest values grow with `nr-io-queues`, so the output also lists them for other queue counts.

## How the memory is calculated

    memory size = roundup(heap x 1.10, 128 MiB)

    heap = fixed(cores) + large pool + small pool + front end + back end

Struct sizes are `sizeof()` in the instance-manager `spdk_tgt` (built with `--enable-debug`, so gdb can print them). Pool
and ring sizes follow `rte_mempool_calc_obj_size` and `rte_ring_create` of the DPDK in the SPDK tree. Only memory that
`spdk_tgt` takes from the hugepages is counted.

| Part | Derivation |
|---|---|
| `bdev_io` pool | 65535 x 1600 B + ring. `spdk_bdev_io` 1216 + the largest driver context (`ftl_io` 280) + 64 B header, rounded to 25 cache lines (the number of lines must be coprime with 4 memory channels) |
| message pool | 262143 x 192 B + ring |
| `fsdev_io` pool | 65535 x 576 B + ring |
| event pool | 16383 x 192 B + ring + 128 lcore caches |
| rings per core | 2 x 1 MiB: the event ring of the reactor and the message ring of the thread. `rte_ring_create` rounds `65536 + 1` entries up to 131072 slots |
| 1 MiB buffers | 3: the bdev zero buffer and the FTL write and read buffers |
| iobuf pools | count x buffer size (132 KiB or 8 KiB) + ring |
| target queue pair | 128 x 4096 B in-capsule buffers + 257 PDUs x 1088 B rounded to 4 KiB, about 788 KiB (`nvmf_tcp_qpair_init_mem_resource`) |
| initiator I/O queue pair | 130 PDUs x 1088 B rounded to 4 KiB + 512 requests x 320 B, about 300 KiB (`nvme_tcp_alloc_reqs`) |
| initiator admin queue pair | 34 PDUs + 32 requests, about 50 KiB |

Queue pair counts:

- Front end: `V x (nr-io-queues + 1)` target queue pairs (I/O plus admin).
- Back end: `R x (cores + 1)` target queue pairs, plus `R x (cores I/O + 1 admin)` initiator queue pairs, because every
  replica is connected to one engine. On one node the two sides are assumed to be equal.
- Only DMA memory is in the hugepages. `calloc` memory, such as the request array of the target, is not.

Blob metadata is not counted. The cluster arrays of a blob use `realloc` (`lib/blob/blobstore.c`), which is process heap,
not hugepages, so the size and snapshots of the volumes do not change `data-engine-memory-size`.

## Validation

The model was compared with the heap reported by `env_dpdk_get_mem_stats` on a 3-node NVMe cluster (instance-manager
`v1.12.x-head`, 6 GiB volumes):

- 12 configurations with volumes (4, 8 and 16 cores, 16 to 50 replicas, 1 to 4 queues, large pools 2048 to 9216, small
  pools 8192 to 16384) and 4 idle heaps (2, 4, 8 and 16 cores).
- The model is below every measurement, by 0.4% to 0.9% (up to 1.2% in the test tolerance).
- The mempool and ring sizes match the memory zones of the dump to within 0.06 MiB.
- The cache-fit minimum matched 646 stand-alone `spdk_tgt` runs on 1 to 20 cores.

The tests are in `dataengine/calculator_test.go` (`TestHeapAgainstMeasurements`, `TestMempoolsAgainstTheMemoryZones`,
`TestCacheFitMinimum` and others).

## Limits

- About 3 MiB of the idle heap (four allocations of 810944 bytes) is not traced to the source. The 10% headroom covers it.
- The highest pools are a sufficient condition from the source, not a reproduced failure threshold.
- Between at least and highest, the result depends on the workload, which the command does not know. Use the highest
  values when the memory allows.
- The effect of a smaller `nr-io-queues` on the throughput of a single volume was not measured.
- The ublk caches come from the source and were not tested (no `ublk_drv` on the test nodes).
- The estimate assumes a single NVMe/TCP transport. Two transports reserve twice the caches and make `spdk_tgt` fail at
  any pool size.
- The memory comparison was done on one cluster only.
