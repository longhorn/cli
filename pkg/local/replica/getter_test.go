package replica

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type GetterTestSuite struct {
	suite.Suite
}

func (s *GetterTestSuite) TestGetVolumeNameFromReplicaDirectoryName() {
	testCases := []struct {
		replicaDirectoryName string
		expectedVolumeName   string
		expectedOk           bool
	}{
		{"pvc-48a6457d-585e-423b-b530-bbc68a5f948a-0e2603a7", "pvc-48a6457d-585e-423b-b530-bbc68a5f948a", true},
		{"vol-1a2b3c4d", "vol", true},
		{"no-hyphen-suffix-", "no-hyphen-suffix", true},
		{"nohyphen", "", false},
		{"-1a2b3c4d", "", false},
		{"", "", false},
	}

	for _, testCase := range testCases {
		volumeName, ok := getVolumeNameFromReplicaDirectoryName(testCase.replicaDirectoryName)
		s.Equal(testCase.expectedOk, ok, "input: %q", testCase.replicaDirectoryName)
		s.Equal(testCase.expectedVolumeName, volumeName, "input: %q", testCase.replicaDirectoryName)
	}
}

func TestGetterTestSuite(t *testing.T) {
	suite.Run(t, new(GetterTestSuite))
}
