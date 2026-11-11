package types

import "fmt"

type DataEngineMode string

const (
	DataEngineModeV1Capable DataEngineMode = "v1-capable"
	DataEngineModeV2Only    DataEngineMode = "v2-only"
)

func ParseDataEngineMode(value string) (DataEngineMode, error) {
	switch DataEngineMode(value) {
	case "", DataEngineModeV1Capable:
		return DataEngineModeV1Capable, nil
	case DataEngineModeV2Only:
		return DataEngineModeV2Only, nil
	default:
		return "", fmt.Errorf("data-engine-mode must be v1-capable or v2-only")
	}
}

func (mode DataEngineMode) RequiresISCSI() bool { return mode != DataEngineModeV2Only }

func (mode DataEngineMode) NeedsSPDK(explicit bool) bool {
	return explicit || mode == DataEngineModeV2Only
}
