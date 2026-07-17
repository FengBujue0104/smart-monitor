package ui

import (
	"strings"
	"testing"

	"smonitor/health"
	"smonitor/smart"
)

func TestAttrDisplaySeparatesRawAndNormalizedValues(t *testing.T) {
	ataTemp := smart.Attr{ID: 0xC2, Raw: 0x46302A, Value: 100, Worst: 90, Kind: "ata"}
	if got := attrRawStr(ataTemp); got != "42°C (min 48°C, max 70°C)" {
		t.Fatalf("unexpected ATA raw display: %q", got)
	}
	if got := attrFlagsStr(smart.Attr{Flags: 0x35, Kind: "ata"}); got != "0x0035" {
		t.Fatalf("unexpected ATA flags display: %q", got)
	}
	if got := attrCurrentStr(ataTemp); got != "100" || attrWorstStr(ataTemp) != "90" {
		t.Fatalf("unexpected ATA normalized display: current=%q worst=%q", got, attrWorstStr(ataTemp))
	}

	nvmeTemp := smart.Attr{ID: smart.NVMeTemperature, Raw: 300, Kind: "nvme"}
	if got := attrRawStr(nvmeTemp); got != "27°C" {
		t.Fatalf("unexpected NVMe temperature display: %q", got)
	}
	if attrCurrentStr(nvmeTemp) != "-" || attrWorstStr(nvmeTemp) != "-" {
		t.Fatal("NVMe should not display ATA normalized columns")
	}
	if got := attrRawStr(smart.Attr{ID: smart.NVMeTemperature, Kind: "nvme"}); got != "N/A" {
		t.Fatalf("unexpected unavailable NVMe temperature display: %q", got)
	}

	nvmeWrite := smart.Attr{ID: smart.NVMeDataUnitsWritten, Raw: 1000000, Kind: "nvme"}
	if got := attrRawStr(nvmeWrite); !strings.Contains(got, "0.51 TB") || !strings.Contains(got, "1000000 units") {
		t.Fatalf("unexpected NVMe data unit display: %q", got)
	}

	nvmeHighCounter := smart.Attr{ID: smart.NVMeMediaErrors, RawHigh: 1, Kind: "nvme"}
	if got := attrRawStr(nvmeHighCounter); got != "0x00000000000000010000000000000000" {
		t.Fatalf("unexpected 128-bit NVMe counter display: %q", got)
	}
}

func TestDiskSummaryIncludesCapacityAndReadState(t *testing.T) {
	d := smart.Disk{Model: "Test SSD", SizeGB: 1024, Attrs: []smart.Attr{{ID: 1}}}
	got := diskSummary(d)
	if !strings.Contains(got, "1024 GB") || !strings.Contains(got, "SMART数据已读取") {
		t.Fatalf("unexpected disk summary: %q", got)
	}
}

func TestDiskSummaryShowsOverallSMARTStatusWhenKnown(t *testing.T) {
	d := smart.Disk{Model: "Test HDD", SmartStatusKnown: true, SmartStatusPassed: false, Attrs: []smart.Attr{{ID: 1}}}
	if got := diskSummary(d); !strings.Contains(got, "SMART失败") || smartStatusText(d) != "失败" {
		t.Fatalf("unexpected failed SMART summary: %q", got)
	}
}

func TestDiskSummaryShowsChecksumWarning(t *testing.T) {
	d := smart.Disk{Model: "Test HDD", SMARTChecksumKnown: true, SMARTChecksumValid: false, Attrs: []smart.Attr{{ID: 1}}}
	if got := diskSummary(d); !strings.Contains(got, "校验和异常") || smartChecksumText(d) != "异常" {
		t.Fatalf("unexpected checksum summary: %q", got)
	}
}

func TestDiskSummaryShowsThresholdChecksumWarning(t *testing.T) {
	d := smart.Disk{Model: "Test HDD", SMARTThresholdChecksumKnown: true, SMARTThresholdChecksumValid: false, Attrs: []smart.Attr{{ID: 1}}}
	if got := diskSummary(d); !strings.Contains(got, "校验和异常") || smartThresholdChecksumText(d) != "异常" {
		t.Fatalf("unexpected threshold checksum summary: %q", got)
	}
}

func TestReportModelShowsDiskWhenSMARTDataIsUnavailable(t *testing.T) {
	m := &reportModel{disks: []smart.Disk{{Index: 4, Kind: smart.KindATA, Model: "Unavailable"}}}
	if got := m.RowCount(); got != 1 {
		t.Fatalf("row count = %d, want 1", got)
	}
	if got := m.Value(0, 2); got != "-" {
		t.Fatalf("unavailable SMART ID = %q, want -", got)
	}
	if got := m.Value(0, 4); got != "SMART 数据未读取" {
		t.Fatalf("unavailable SMART row name = %q", got)
	}
}

func TestTextReportDoesNotClaimHealthyWhenSMARTIsUnavailable(t *testing.T) {
	report := buildTextReport([]smart.Disk{{Index: 1, Kind: smart.KindATA, Model: "Unavailable"}}, nil)
	if !strings.Contains(report, "无法得出完整健康结论") || strings.Contains(report, "所有监测指标在安全范围内") {
		t.Fatalf("unexpected text report conclusion: %q", report)
	}
}

func TestReportModelShowsDiskLevelSMARTDiagnostics(t *testing.T) {
	m := &reportModel{
		disks: []smart.Disk{{Index: 3, Kind: smart.KindATA, Model: "Failed", Attrs: []smart.Attr{{ID: 5}}}},
		violations: []health.Violation{
			{DiskIndex: 3, AttrID: 0, AttrName: "SMART_Overall_Health", Current: "FAILED", Limit: "PASSED", Severity: health.Critical},
			{DiskIndex: 3, AttrID: -1, AttrName: "SMART_Data_Checksum", Current: "INVALID", Limit: "VALID", Severity: health.Warning},
		},
	}
	if got := m.RowCount(); got != 3 {
		t.Fatalf("row count = %d, want 3", got)
	}
	if got := m.Value(0, 4); got != "SMART_Overall_Health" || m.Value(0, 9) != "❌ 严重" {
		t.Fatalf("unexpected overall diagnostic row: name=%q status=%q", got, m.Value(0, 9))
	}
	if got := m.Value(1, 2); got != "-" || m.Value(1, 4) != "SMART_Data_Checksum" || m.Value(1, 9) != "⚠️ 警告" {
		t.Fatalf("unexpected checksum diagnostic row: id=%q name=%q status=%q", got, m.Value(1, 4), m.Value(1, 9))
	}
}
