package common

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"golang.org/x/sys/unix"
)

func Confirm(prompt string) bool {
	fmt.Printf("\n%s [y/N]: ", prompt)
	reader := bufio.NewReader(os.Stdin)
	answer, _ := reader.ReadString('\n')
	answer = strings.TrimSpace(strings.ToLower(answer))
	return answer == "y" || answer == "yes"
}

// BlockSize returns the filesystem block size backing given file.
func BlockSize(file *os.File) (int64, error) {
	var st unix.Statfs_t
	if err := unix.Fstatfs(int(file.Fd()), &st); err != nil {
		return 0, fmt.Errorf("failed to statfs: %w", err)
	}
	return int64(st.Bsize), nil
}

func EstimateReclaimable(offset, length, blockSize int64) int64 {
	end := offset + length
	alignedStart := (offset + blockSize - 1) / blockSize * blockSize
	alignedEnd := end / blockSize * blockSize

	if alignedEnd <= alignedStart {
		return 0
	}
	return alignedEnd - alignedStart
}
