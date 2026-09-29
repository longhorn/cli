package diagnose

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/longhorn/cli/pkg/types"
)

func TestResultsMarshal(t *testing.T) {
	results := Results{
		{Name: "LonghornManager", Status: StatusPass, LogCollection: types.LogCollection{Info: []string{"ok"}}},
		{Name: "Volumes", Status: StatusFail, LogCollection: types.LogCollection{Error: []string{"Volume vol-1 is faulted"}}},
	}

	for _, test := range []struct {
		format      string
		expected    string
		expectedErr string
	}{
		{
			format: OutputFormatYAML,
			expected: `- name: LonghornManager
  status: pass
  info:
  - ok
- name: Volumes
  status: fail
  error:
  - Volume vol-1 is faulted
`,
		},
		{
			format: OutputFormatJSON,
			expected: `[
  {
    "name": "LonghornManager",
    "status": "pass",
    "info": [
      "ok"
    ]
  },
  {
    "name": "Volumes",
    "status": "fail",
    "error": [
      "Volume vol-1 is faulted"
    ]
  }
]
`,
		},
		{
			format:      "table",
			expectedErr: `unsupported output format "table", must be one of [yaml json]`,
		},
	} {
		t.Run(test.format, func(t *testing.T) {
			output, err := results.Marshal(test.format)
			if test.expectedErr != "" {
				assert.EqualError(t, err, test.expectedErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, test.expected, string(output))
		})
	}
}

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
