package main

import (
	"strings"
	"testing"

	"smonitor/health"
	"smonitor/smart"
	"smonitor/ui"
)

// TestSimulatedDiskFailuresProduceFeedbackReport is a hardware-independent
// end-to-end failure fixture. It exercises ATA and NVMe health evaluation and
// the concise exception-only report copied by the GUI button.
func TestSimulatedDiskFailuresProduceFeedbackReport(t *testing.T) {
	disks := []smart.Disk{
		{
			Index: 10,
			Kind:  smart.KindATA,
			Model: "SIMULATED ATA FAILURE",
			Attrs: []smart.Attr{
				{ID: 0x05, Name: "Reallocated_Sector_Ct", Raw: 2, Kind: "ata"},
				{ID: 0xC2, Name: "Temperature_Celsius", Raw: 61, Kind: "ata"},
				{ID: 0xBC, Name: "Command_Timeout", Raw: 11, Kind: "ata"},
				{ID: 0xAA, Name: "Vendor_PreFail", Flags: 0x0001, Value: 10, Thresh: 10, Kind: "ata"},
				{ID: 0x09, Name: "Power_On_Hours", Raw: 1000, Kind: "ata"}, // healthy control value
			},
		},
		{
			Index: 11,
			Kind:  smart.KindNVMe,
			Model: "SIMULATED NVME FAILURE",
			Attrs: []smart.Attr{
				{ID: smart.NVMeCriticalWarning, Name: "Critical_Warning", Raw: 0x02, Kind: "nvme"},
				{ID: smart.NVMeMediaErrors, Name: "Media_Data_Integrity_Errors", Raw: 1, Kind: "nvme"},
				{ID: smart.NVMeTemperatureSensor1, Name: "Temperature_Sensor_1_Kelvin", Raw: 334, Kind: "nvme"},
				{ID: smart.NVMePercentUsed, Name: "Percentage_Used", Raw: 80, Kind: "nvme"},
			},
		},
	}

	violations := health.Evaluate(disks)
	if len(violations) != 8 {
		t.Fatalf("violation count = %d, want 8: %+v", len(violations), violations)
	}
	var critical, warning int
	for _, v := range violations {
		switch v.Severity {
		case health.Critical:
			critical++
		case health.Warning:
			warning++
		}
	}
	if critical != 6 || warning != 2 {
		t.Fatalf("severity counts critical=%d warning=%d, want 6/2: %+v", critical, warning, violations)
	}

	report := ui.BuildExceptionReportForTest(disks, violations)
	if !strings.HasPrefix(report, "磁盘号\t型号\t异常项目\t当前值\t阈值\t级别\n") {
		t.Fatalf("missing TSV header: %q", report)
	}
	if !strings.Contains(report, "Disk10\tSIMULATED ATA FAILURE") || !strings.Contains(report, "Disk11\tSIMULATED NVME FAILURE") {
		t.Fatalf("missing simulated disks in feedback report: %q", report)
	}
	if strings.Contains(report, "Power_On_Hours") {
		t.Fatalf("healthy attribute must not be copied: %q", report)
	}
	if lines := strings.Split(strings.TrimSpace(report), "\n"); len(lines) != len(violations)+1 {
		t.Fatalf("feedback rows = %d, want %d: %q", len(lines)-1, len(violations), report)
	}
}
