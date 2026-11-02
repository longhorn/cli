package preflight

import (
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func TestCheckerModeRequiresResolvedV2Prerequisites(t *testing.T) {
	for _, test := range []struct {
		name, mode, driver string
		hugepages          int
		valid              bool
	}{
		{"default", "", "", 0, true},
		{"v1", "v1-capable", "", 0, true},
		{"v2", "v2-only", "", 2048, true},
		{"v2-vfio-module", "v2-only", "vfio_pci", 2048, true},
		{"v2-uio-module", "v2-only", "uio_pci_generic", 2048, true},
		{"vfio-driver-is-not-module", "v2-only", "vfio-pci", 2048, false},
		{"invalid-mode", "v2", "", 2048, false},
		{"missing-pages", "v2-only", "", 0, false},
		{"negative-pages", "v2-only", "", -2, false},
		{"unaligned-pages", "v2-only", "", 1, false},
		{"invalid-driver", "v2-only", "unknown", 2048, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			options := CheckerCmdOptions{DataEngineMode: test.mode, HugePageSize: test.hugepages, UserspaceDriver: test.driver}
			if err := options.ValidateDataEngineMode(); (err == nil) != test.valid {
				t.Fatalf("unexpected checker mode validation: %v", err)
			}
			if test.valid && options.DataEngineMode == "" {
				t.Fatal("default capability was not normalized")
			}
		})
	}
}

func TestV2SPDKEnvironmentBindsNativeMemoryPCIAndNUMAInputs(t *testing.T) {
	options := InstallerCmdOptions{DataEngineMode: "v2-only", HugePageSize: 2048, AllowPci: "0000:00:01.0,0000:00:02.0", DriverOverride: "vfio-pci", SpdkOptions: "HUGENODE=1"}
	environment, node, err := options.V2SPDKSetupEnvironment()
	if err != nil || node != "1" {
		t.Fatalf("unresolved setup profile: %v", err)
	}
	want := []string{"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin", "LC_ALL=C", "HUGEMEM=2048", "HUGEPGSZ=2048", "HUGENODE=1", "PCI_ALLOWED=0000:00:01.0 0000:00:02.0", "DRIVER_OVERRIDE=vfio-pci", "TARGET_USER=root"}
	if !reflect.DeepEqual(environment, want) {
		t.Fatalf("setup environment does not bind the resolved options: %v", environment)
	}
	options.AllowPci, options.SpdkOptions = "", ""
	environment, node, err = options.V2SPDKSetupEnvironment()
	if err != nil || node != "0" || !strings.Contains(strings.Join(environment, "\n"), "PCI_ALLOWED=none\n") {
		t.Fatal("empty PCI selection did not remain fenced, or native node-0 default changed")
	}
}

func TestRemoteCheckerBindsCapabilityToTheLocalExecutable(t *testing.T) {
	for _, mode := range []string{"", "v1-capable", "v2-only"} {
		for _, explicitSPDK := range []bool{false, true} {
			checker := Checker{CheckerCmdOptions: CheckerCmdOptions{DataEngineMode: mode, HugePageSize: 2048, EnableSpdk: explicitSPDK}, appName: "checker"}
			if err := checker.ValidateDataEngineMode(); err != nil {
				t.Fatal(err)
			}
			pod := checker.newDaemonSet().Spec.Template.Spec
			container := pod.InitContainers[0]
			environment := map[string]string{}
			for _, variable := range container.Env {
				environment[variable.Name] = variable.Value
			}
			if environment["DATA_ENGINE_MODE"] != checker.DataEngineMode || environment["ENABLE_SPDK"] != strconv.FormatBool(explicitSPDK) {
				t.Fatal("remote job lost capability or overwrote the explicit SPDK flag")
			}
			command := strings.Join(append(append([]string{}, container.Command...), container.Args...), " ")
			if strings.Contains(command, "--data-engine-mode v2-only") != (mode == "v2-only") {
				t.Fatal("V2-only job can silently run an older local executable, or default argv changed")
			}
		}
	}
}

func TestRemoteCheckerRejectsInvalidModeBeforeConnectionInitialization(t *testing.T) {
	checker := Checker{CheckerCmdOptions: CheckerCmdOptions{DataEngineMode: "unknown"}}
	if err := checker.Init(); err == nil || !strings.Contains(err.Error(), "data-engine-mode") {
		t.Fatal("invalid capability reached connection initialization")
	}
}

func TestRemoteInstallerBindsV2ExecutableBeforeAnyMutation(t *testing.T) {
	installer := Installer{InstallerCmdOptions: InstallerCmdOptions{DataEngineMode: "v2-only", HugePageSize: 2048}, appName: "installer"}
	if err := installer.ValidateDataEngineMode(); err != nil {
		t.Fatal(err)
	}
	container := installer.newDaemonSetForPackageManager().Spec.Template.Spec.InitContainers[0]
	if !strings.Contains(strings.Join(container.Command, " "), "--data-engine-mode v2-only") {
		t.Fatal("V2 installer may silently execute an older V1-capable image")
	}
	if err := installer.InstallByContainerOptimizedOS(); err == nil {
		t.Fatal("V2 installer reached unsupported COS mutations")
	}
}

func TestRemoteInstallerRejectsInvalidModeBeforeConnectionInitialization(t *testing.T) {
	installer := Installer{InstallerCmdOptions: InstallerCmdOptions{DataEngineMode: "unknown"}}
	if err := installer.Init(); err == nil || !strings.Contains(err.Error(), "data-engine-mode") {
		t.Fatal("invalid mode reached remote installer initialization")
	}
}

func TestInstallerModeRejectsUnqualifiedOrConflictingSetupBeforeWrites(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*InstallerCmdOptions)
		valid  bool
	}{
		{"compatible", func(*InstallerCmdOptions) {}, true},
		{"invalid-mode", func(o *InstallerCmdOptions) { o.DataEngineMode = "v2" }, false},
		{"cos", func(o *InstallerCmdOptions) { o.OperatingSystem = "cos" }, false},
		{"talos", func(o *InstallerCmdOptions) { o.OperatingSystem = "talos" }, false},
		{"zero-pages", func(o *InstallerCmdOptions) { o.HugePageSize = 0 }, false},
		{"unaligned-pages", func(o *InstallerCmdOptions) { o.HugePageSize = 3 }, false},
		{"bad-window", func(o *InstallerCmdOptions) { o.RestartKubelet, o.RestartKubeletWindow = true, "invalid" }, false},
		{"negative-window", func(o *InstallerCmdOptions) { o.RestartKubelet, o.RestartKubeletWindow = true, "-1m" }, false},
		{"invalid-driver", func(o *InstallerCmdOptions) { o.DriverOverride = "none" }, false},
		{"invalid-pci", func(o *InstallerCmdOptions) { o.AllowPci = "$(untrusted)" }, false},
		{"conflicting-pages", func(o *InstallerCmdOptions) { o.SpdkOptions = "HUGEMEM=0" }, false},
		{"conflicting-size", func(o *InstallerCmdOptions) { o.SpdkOptions = "HUGEPGSZ=1048576" }, false},
		{"override-page-count", func(o *InstallerCmdOptions) { o.SpdkOptions = "NRHUGE=0" }, false},
		{"unknown-option", func(o *InstallerCmdOptions) { o.SpdkOptions = "PERSIST_HUGE=yes" }, false},
		{"duplicate-option", func(o *InstallerCmdOptions) { o.SpdkOptions = "HUGEMEM=2048,HUGEMEM=2048" }, false},
		{"eval-node", func(o *InstallerCmdOptions) { o.SpdkOptions = "HUGENODE=$(untrusted)" }, false},
		{"resolved-options", func(o *InstallerCmdOptions) { o.SpdkOptions = "HUGEMEM=2048,HUGEPGSZ=2048,HUGENODE=0" }, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			options := InstallerCmdOptions{DataEngineMode: "v2-only", HugePageSize: 2048, AllowPci: "none"}
			test.mutate(&options)
			if err := options.ValidateDataEngineMode(); (err == nil) != test.valid {
				t.Fatalf("unexpected installer profile validation: %v", err)
			}
		})
	}
	options := InstallerCmdOptions{SpdkOptions: "legacy-unparsed", OperatingSystem: "cos"}
	if err := options.ValidateDataEngineMode(); err != nil || options.DataEngineMode != "v1-capable" {
		t.Fatal("default installation behavior changed")
	}
}
