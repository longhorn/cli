package diagnose

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDiagnoserChecks(t *testing.T) {
	expected := []string{
		"LonghornManager",
		"LonghornBackend",
		"LonghornNodes",
		"EngineImages",
		"InstanceManagers",
		"CSIDriver",
		"CSIPlugin",
		"CSIAttacher",
		"CSIProvisioner",
		"CSIResizer",
		"CSISnapshotter",
		"LonghornUI",
		"Volumes",
	}

	diagnoser := &Diagnoser{}

	names := []string{}
	for _, check := range diagnoser.checks() {
		assert.NotNil(t, check.run, "check %v has no function", check.name)
		names = append(names, check.name)
	}
	assert.Equal(t, expected, names)
}
