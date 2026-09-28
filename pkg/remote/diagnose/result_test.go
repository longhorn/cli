package diagnose

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/longhorn/cli/pkg/types"
)

func TestResultUpdateStatus(t *testing.T) {
	for _, test := range []struct {
		name     string
		logs     types.LogCollection
		expected Status
	}{
		{
			name:     "no messages",
			expected: StatusPass,
		},
		{
			name:     "info",
			logs:     types.LogCollection{Info: []string{"info"}},
			expected: StatusPass,
		},
		{
			name:     "warn",
			logs:     types.LogCollection{Info: []string{"info"}, Warn: []string{"warn"}},
			expected: StatusWarn,
		},
		{
			name:     "error",
			logs:     types.LogCollection{Info: []string{"info"}, Warn: []string{"warn"}, Error: []string{"error"}},
			expected: StatusFail,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			result := &Result{LogCollection: test.logs}
			result.updateStatus()
			assert.Equal(t, test.expected, result.Status)
		})
	}
}

func TestResultsFailed(t *testing.T) {
	results := Results{
		{Name: "A", Status: StatusPass},
		{Name: "B", Status: StatusFail},
		{Name: "C", Status: StatusWarn},
		{Name: "D", Status: StatusFail},
	}
	assert.Equal(t, []string{"B", "D"}, results.Failed())

	assert.Empty(t, Results{{Name: "A", Status: StatusPass}}.Failed())
	assert.Empty(t, Results{}.Failed())
}
