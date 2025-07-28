package preflight

import (
	"os/exec"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/suite"
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
