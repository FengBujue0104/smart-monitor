package main

import (
	"strings"
	"testing"

	"smonitor/health"
	"smonitor/ui"
)

// TestSimulatedDiskFailuresProduceFeedbackReport is a hardware-independent
// end-to-end failure fixture. It exercises ATA and NVMe health evaluation and
// the concise exception-only report copied by the GUI button.
func TestSimulatedDiskFailuresProduceFeedbackReport(t *testing.T) {
	disks := ui.SimulatedFailureDisks()

	violations := health.Evaluate(disks)
	if len(violations) != 9 {
		t.Fatalf("violation count = %d, want 9: %+v", len(violations), violations)
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
	// 剩余寿命规则为“低于 50% 即红色告警”：E6(5%) 与 NVMe PercentUsed(80%)
	// 均为 critical，复合温度 334K(61°C) 也是 critical，只有 BC(Command_Timeout)
	// 是 warning。
	if critical != 8 || warning != 1 {
		t.Fatalf("severity counts critical=%d warning=%d, want 8/1: %+v", critical, warning, violations)
	}

	report := ui.BuildExceptionReportForTest(disks, violations)
	if !strings.HasPrefix(report, "磁盘号\t型号\t异常项目\t当前值\t阈值\t级别\n") {
		t.Fatalf("missing TSV header: %q", report)
	}
	if !strings.Contains(report, "Disk10\tSIMULATED WD Blue SA510 FAILURE") || !strings.Contains(report, "5% (remaining)") || !strings.Contains(report, "Disk11\tSIMULATED NVME FAILURE") {
		t.Fatalf("missing simulated disks in feedback report: %q", report)
	}
	if strings.Contains(report, "Power_On_Hours") {
		t.Fatalf("healthy attribute must not be copied: %q", report)
	}
	if lines := strings.Split(strings.TrimSpace(report), "\n"); len(lines) != len(violations)+1 {
		t.Fatalf("feedback rows = %d, want %d: %q", len(lines)-1, len(violations), report)
	}
}
