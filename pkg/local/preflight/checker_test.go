package preflight

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	"github.com/longhorn/cli/pkg/local/preflight/packagemanager/mocks"
	"github.com/longhorn/cli/pkg/types"
)

type UtilTestSuite struct {
	suite.Suite
}

func (s *UtilTestSuite) SetupTest() {
	// Setup placeholder — can be used for initializing shared state
}

func (s *UtilTestSuite) TestWrapMsgWithTopic() {
	s.Equal("Topic This is a message", wrapMsgWithTopic("Topic", "This is a message"))
}

func (s *UtilTestSuite) TestFormatTopic() {
	s.Equal("[A][B]", formatTopic("A", "B"))
	s.Equal("", formatTopic())
}

func (s *UtilTestSuite) TestWrapMultItems() {
	// === Case 1: Normal error values ===
	itemsWithErrors := map[string]any{
		"nvme-cli":  errors.New("command not found"),
		"sg3_utils": errors.New("exit status 1"),
	}

	result := wrapMultItems("The following packages are not installed:", itemsWithErrors)

	s.Contains(result, "The following packages are not installed:")
	s.Contains(result, "nvme-cli: command not found")
	s.Contains(result, "sg3_utils: exit status 1")

	// === Case 2: Nil value ===
	itemsWithNil := map[string]any{
		"some-key": nil,
	}

	expected := "Missing items:  (1) some-key"
	result = wrapMultItems("Missing items:", itemsWithNil)
	s.Equal(expected, result)
}

func (s *UtilTestSuite) TestWrapInternalError() {
	err := wrapInternalError("Topic", errors.New("boom"))
	s.Error(err)
	s.Contains(err.Error(), "Topic[InternalError]")
	s.Contains(err.Error(), "boom")
}

func (s *UtilTestSuite) TestWrapAggregatedInternalError() {
	items := map[string]any{
		"dep": errors.New("fail"),
	}
	err := wrapAggregatedInternalError("Engine", "Missing deps:", items)
	s.Error(err)
	s.Contains(err.Error(), "Engine[InternalError]")
	s.Contains(err.Error(), "dep: fail")
}

func (s *UtilTestSuite) TestIsExitCode() {
	cmd := exec.Command("sh", "-c", "exit 42")
	err := cmd.Run()
	s.True(isExitCode(err, 42))
	s.False(isExitCode(err, 1))

	nonExitErr := errors.New("generic error")
	s.False(isExitCode(nonExitErr, 1))
}

func TestUtils(t *testing.T) {
	suite.Run(t, new(UtilTestSuite))
}

func TestCheckIOMMUSupport(t *testing.T) {
	tests := []struct {
		name          string
		execOutput    string
		execErr       error
		wantErr       bool
		wantInfoCount int
		wantErrCount  int
		wantLogSubstr string
	}{
		{
			name:          "IOMMU is enabled and groups found",
			execOutput:    "/sys/kernel/iommu_groups/1\n/sys/kernel/iommu_groups/2\n",
			execErr:       nil,
			wantErr:       false,
			wantInfoCount: 1,
			wantErrCount:  0,
			wantLogSubstr: "IOMMU is enabled (2 IOMMU groups found)",
		},
		{
			name:          "IOMMU is disabled",
			execOutput:    "",
			execErr:       nil,
			wantErr:       false,
			wantInfoCount: 0,
			wantErrCount:  1,
			wantLogSubstr: "IOMMU is not enabled: no groups found under /sys/kernel/iommu_groups",
		},
		{
			name:          "execute fails",
			execOutput:    "",
			execErr:       errors.New("exec failed"),
			wantErr:       true,
			wantInfoCount: 0,
			wantErrCount:  1,
			wantLogSubstr: "failed to check IOMMU groups",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockPM := new(mocks.MockPackageManager)
			mockPM.On("Execute",
				[]string{}, "sh", mock.Anything, mock.Anything,
			).Return(tc.execOutput, tc.execErr).Once()

			checker := &Checker{
				packageManager: mockPM,
				collection: types.NodeCollection{
					Log: &types.LogCollection{},
				},
			}

			err := checker.checkIOMMUSupport()

			if tc.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantLogSubstr)
			} else {
				assert.NoError(t, err)
				assert.Len(t, checker.collection.Log.Info, tc.wantInfoCount)
				assert.Len(t, checker.collection.Log.Error, tc.wantErrCount)

				if tc.wantInfoCount > 0 {
					combinedInfoLogs := strings.Join(checker.collection.Log.Info, "\n")
					assert.Contains(t, combinedInfoLogs, tc.wantLogSubstr)
				}
				if tc.wantErrCount > 0 {
					combinedErrLogs := strings.Join(checker.collection.Log.Error, "\n")
					assert.Contains(t, combinedErrLogs, tc.wantLogSubstr)
				}
			}
			mockPM.AssertExpectations(t)
		})
	}
}
