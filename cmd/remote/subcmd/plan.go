package subcmd

import (
	"os"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/longhorn/cli/pkg/consts"
	"github.com/longhorn/cli/pkg/plan/dataengine"
	"github.com/longhorn/cli/pkg/utils"
)

const (
	cmdOptCPUCores         = "cpu-cores"
	cmdOptCPUMask          = "cpu-mask"
	cmdOptNumberOfEngines  = "number-of-engines"
	cmdOptNumberOfReplicas = "number-of-replicas"
	cmdOptNrIoQueues       = "nr-io-queues"
	cmdOptHostCPUs         = "host-cpus"
	cmdOptUblkCores        = "ublk-cores"
)

type planOptions struct {
	dataEngine string
	cpuMask    string
	output     string
	input      dataengine.Input
}

// NewCmdPlan returns the plan command, which plans Longhorn settings from the cluster layout.
func NewCmdPlan() *cobra.Command {
	opts := &planOptions{input: dataengine.DefaultInput()}

	cmd := &cobra.Command{
		Use:   consts.SubCmdPlan,
		Args:  cobra.NoArgs,
		Short: "Plan Longhorn data engine settings for a cluster",
		Long: `Calculate data-engine-memory-size, data-engine-iobuf-large-pool-size and data-engine-iobuf-small-pool-size
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
the instance manager at any pool size.`,
		Example: `  # A node with 17 engines and 50 replicas, 8 SPDK cores
  longhornctl plan --data-engine v2 --cpu-cores 8 --number-of-engines 17 --number-of-replicas 50

  # The same, with the data-engine-cpu-mask of the setting and JSON output
  longhornctl plan --data-engine v2 --cpu-mask 0xFF --number-of-engines 17 --number-of-replicas 50 -o json

  # With 2 NVMe I/O queues per volume instead of the default 4
  longhornctl plan --data-engine v2 --cpu-cores 8 --number-of-engines 17 --number-of-replicas 50 --nr-io-queues 2`,

		Run: func(cmd *cobra.Command, args []string) {
			utils.CheckErr(runPlan(cmd, opts))
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&opts.dataEngine, consts.CmdOptDataEngine, "", "Data engine to calculate for (v2)")
	flags.IntVar(&opts.input.Cores, cmdOptCPUCores, 0, "Number of SPDK cores (bits of data-engine-cpu-mask). Use either this or --"+cmdOptCPUMask)
	flags.StringVar(&opts.cpuMask, cmdOptCPUMask, "", "Value of data-engine-cpu-mask, for example 0xFF. Use either this or --"+cmdOptCPUCores)
	flags.IntVar(&opts.input.Engines, cmdOptNumberOfEngines, 0, "Number of engines on a node, the node with the most: the volumes attached to it")
	flags.IntVar(&opts.input.Replicas, cmdOptNumberOfReplicas, 0, "Number of replicas on a node, the node with the most, of any volume")
	flags.IntVar(&opts.input.NrIoQueues, cmdOptNrIoQueues, opts.input.NrIoQueues, "Value of default-nvme-tcp-nr-io-queues. 0 is the kernel default, one queue for each host CPU, and needs --"+cmdOptHostCPUs)
	flags.IntVar(&opts.input.HostCPUs, cmdOptHostCPUs, 0, "CPUs of a node, used only when --"+cmdOptNrIoQueues+" is 0")
	flags.IntVar(&opts.input.UblkCores, cmdOptUblkCores, 0, "Cores of the ublk target, 0 when the ublk frontend is not used")
	flags.StringVarP(&opts.output, consts.CmdOptOutput, "o", dataengine.OutputFormatText, "Output format (text, json, yaml)")

	return cmd
}

func runPlan(cmd *cobra.Command, opts *planOptions) error {
	switch opts.dataEngine {
	case "v2":
	case "v1":
		return errors.Errorf("--%s v1 is not supported: the V1 data engine has no iobuf pools or SPDK memory to size", consts.CmdOptDataEngine)
	default:
		return errors.Errorf("--%s is required and must be v2", consts.CmdOptDataEngine)
	}

	if cmd.Flags().Changed(cmdOptCPUCores) == cmd.Flags().Changed(cmdOptCPUMask) {
		return errors.Errorf("exactly one of --%s and --%s is required", cmdOptCPUCores, cmdOptCPUMask)
	}

	if cmd.Flags().Changed(cmdOptCPUMask) {
		cores, err := dataengine.ParseCPUMask(opts.cpuMask)
		if err != nil {
			return errors.Wrapf(err, "invalid --%s", cmdOptCPUMask)
		}
		opts.input.Cores = cores
	}

	result, err := dataengine.Calculate(opts.input)
	if err != nil {
		return err
	}

	return dataengine.Render(os.Stdout, result, opts.output)
}
