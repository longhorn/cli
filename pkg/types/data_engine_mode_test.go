package types

import "testing"

func TestDataEngineModePreservesDefaultAndRequiresExplicitV2(t *testing.T) {
	for _, test := range []struct {
		value string
		want  DataEngineMode
	}{
		{"", DataEngineModeV1Capable}, {"v1-capable", DataEngineModeV1Capable}, {"v2-only", DataEngineModeV2Only},
		{"v2", ""}, {"V2-Only", ""}, {" v2-only", ""}, {"unknown", ""},
	} {
		t.Run(test.value, func(t *testing.T) {
			mode, err := ParseDataEngineMode(test.value)
			if test.want == "" {
				if err == nil {
					t.Fatal("invalid capability silently selected a default")
				}
				return
			}
			if err != nil || mode != test.want {
				t.Fatalf("mode=%q err=%v want=%q", mode, err, test.want)
			}
			for _, explicit := range []bool{false, true} {
				if mode.RequiresISCSI() != (mode == DataEngineModeV1Capable) || mode.NeedsSPDK(explicit) != (explicit || mode == DataEngineModeV2Only) {
					t.Fatal("iSCSI or additive SPDK capability changed")
				}
			}
		})
	}
}
