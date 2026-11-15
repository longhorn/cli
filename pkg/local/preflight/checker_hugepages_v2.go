package preflight

import (
	"fmt"
	"strconv"
	"strings"
)

func (local *Checker) checkV2HugePages() error {
	output, err := local.packageManager.Execute([]string{}, "cat", []string{"/sys/kernel/mm/hugepages/hugepages-2048kB/nr_hugepages"}, v2SPDKReadTimeout)
	if err != nil {
		return fmt.Errorf("failed to read actual V2 2MiB allocation: %w", err)
	}
	value := strings.TrimSpace(output)
	pages, err := strconv.ParseUint(value, 10, 64)
	if err != nil || value == "" || strconv.FormatUint(pages, 10) != value {
		return fmt.Errorf("actual V2 2MiB allocation is unresolved")
	}
	if pages < uint64(local.HugePageSize/2) {
		local.collection.Log.Error = append(local.collection.Log.Error, "Actual host 2MiB hugepages are insufficient for V2")
		return nil
	}
	local.collection.Log.Info = append(local.collection.Log.Info, "Verified actual host 2MiB hugepage allocation for V2")
	return local.checkHugePagesCapacity()
}
