package dataengine

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"
)

const (
	OutputFormatText = "text"
	OutputFormatJSON = "json"
	OutputFormatYAML = "yaml"
)

// Render writes the result in the given format (text, json or yaml).
func Render(w io.Writer, r *Result, format string) error {
	switch format {
	case OutputFormatText, "":
		return renderText(w, r)
	case OutputFormatJSON:
		encoder := json.NewEncoder(w)
		encoder.SetIndent("", "  ")
		return encoder.Encode(r)
	case OutputFormatYAML:
		encoder := yaml.NewEncoder(w)
		encoder.SetIndent(2)
		defer func() { _ = encoder.Close() }()
		return encoder.Encode(r)
	default:
		return errors.Errorf("unsupported output format %q, use text, json or yaml", format)
	}
}

func renderText(w io.Writer, r *Result) error {
	var err error
	printf := func(format string, args ...any) {
		if err == nil {
			_, err = fmt.Fprintf(w, format, args...)
		}
	}

	in, d := r.Input, r.Derived

	printf("Longhorn V2 data engine sizing for one node\n\n")
	printf("Input\n")
	printf("  SPDK cores:                 %d (%d poll groups)\n", in.Cores, d.PollGroups)
	printf("  Engines / replicas:         %d / %d\n", d.EnginesPerNode, d.ReplicasPerNode)
	printf("  NVMe I/O queues per volume: %d\n", d.QueuesPerVolume)
	printf("  qpairs:                     %d (front end %d + back end %d)\n\n", d.QpairsOfNode, d.FrontendQpairs, d.BackendQpairs)

	printf("Settings                              at least    highest\n")
	printf("  data-engine-iobuf-large-pool-size  %9d  %9d\n", r.LargePool.AtLeast, r.LargePool.Highest)
	printf("  data-engine-iobuf-small-pool-size  %9d  %9d\n", r.SmallPool.AtLeast, r.SmallPool.Highest)
	printf("  data-engine-memory-size (MiB)      %9d  %9d\n\n", r.MemoryMiB.AtLeast, r.MemoryMiB.Highest)

	printf("  at least: every per-thread cache is filled and spdk_tgt starts (the pools are not below %d / %d, the minimum of the settings);\n", LonghornMinLargePool, LonghornMinSmallPool)
	printf("            the caches take the pool and I/O stalls under load.\n")
	printf("  highest:  the engines can hold at most %d commands (%d engines x %d queues x %d), whatever the workload;\n", d.PeakInflight, d.EnginesPerNode, d.QueuesPerVolume, maxQueueEntries)
	printf("            with these pools they can never take every buffer, so the replicas always get one.\n\n")

	printf("Highest values for other numbers of I/O queues per volume (default-nvme-tcp-nr-io-queues)\n")
	printf("  queues  large pool  small pool  memory (MiB)\n")
	for _, row := range r.ByNrIoQueues {
		marker := ""
		if row.Requested {
			marker = "  <- requested"
		}
		printf("  %6d  %10d  %10d  %12d%s\n", row.NrIoQueues, row.LargePool, row.SmallPool, row.MemoryMiB, marker)
	}
	printf("\n")

	printf("Memory estimate (MiB)                                               at least    highest\n")
	printf("  fixed (pools, rings, %d cores):                              %9.1f  %9.1f\n", in.Cores, r.MemoryAtLeast.BaseMiB, r.MemoryHighest.BaseMiB)
	printf("  large pool:                                                 %9.1f  %9.1f\n", r.MemoryAtLeast.LargePoolMiB, r.MemoryHighest.LargePoolMiB)
	printf("  small pool:                                                 %9.1f  %9.1f\n", r.MemoryAtLeast.SmallPoolMiB, r.MemoryHighest.SmallPoolMiB)
	printf("  front-end qpairs:                                           %9.1f  %9.1f\n", r.MemoryAtLeast.FrontendQpairsMiB, r.MemoryHighest.FrontendQpairsMiB)
	printf("  back-end qpairs (both sides):                               %9.1f  %9.1f\n", r.MemoryAtLeast.BackendQpairsMiB, r.MemoryHighest.BackendQpairsMiB)
	printf("  heap:                                                       %9.1f  %9.1f\n", r.MemoryAtLeast.HeapMiB, r.MemoryHighest.HeapMiB)
	printf("  memory size (heap + %d%%, multiple of %d):                    %9d  %9d\n\n", memoryHeadroomPercent, hugepageGranularityMiB, r.MemoryAtLeast.MemoryMiB, r.MemoryHighest.MemoryMiB)

	printf("Calculation\n")
	for i, step := range r.Steps {
		printf("  %d. %s [%s]\n", i+1, step.Name, step.Basis)
		printf("       %s\n", step.Formula)
		printf("       %s\n", step.Values)
	}

	if len(r.Warnings) > 0 {
		printf("\nNotes\n")
		for _, warning := range r.Warnings {
			printf("  - %s\n", warning)
		}
	}
	return err
}
