package ui

import (
	"strings"
	"testing"

	"github.com/lxn/walk"
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
	if got := attrRawStr(smart.Attr{ID: smart.NVMeEnduranceGroupCriticalWarning, Raw: 4, Kind: "nvme"}); got != "0x04" {
		t.Fatalf("unexpected NVMe critical warning display: %q", got)
	}
	if got := attrRawStrForModel("KIOXIA-EXCERIA SATA SSD", smart.Attr{ID: 0xF1, Raw: 24824, Kind: "ata"}); got != "12412 GB (24824 × 512 MB)" {
		t.Fatalf("unexpected KIOXIA host writes display: %q", got)
	}
	if got := attrRawStrForModel("WD Blue SA510 2.5 500GB SSD", smart.Attr{ID: 0xF2, Raw: 13631, Kind: "ata"}); got != "13631 GB" {
		t.Fatalf("unexpected WD Blue host reads display: %q", got)
	}
	if got := attrRawStrForModel("ZHITAI TiPlus5000", smart.Attr{ID: 0xF3, Raw: 0x46302A, Kind: "ata"}); got != "42°C (min 48°C, max 70°C)" {
		t.Fatalf("unexpected YMTC F3 temperature display: %q", got)
	}
	if got := attrRawStrForModel("ZHITAI TiPlus5000", smart.Attr{ID: 0xF1, Raw: 2097152, Kind: "ata"}); got != "1.00 GiB (2097152 × 512 B)" {
		t.Fatalf("unexpected YMTC host writes display: %q", got)
	}
	if got := attrRawStrForModel("Intel SSD DC S3500", smart.Attr{ID: 0xF3, Raw: 42, Kind: "ata"}); got != "42" {
		t.Fatalf("unexpected non-YMTC F3 display: %q", got)
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

func TestAlertBannerTextRepresentsCurrentScanState(t *testing.T) {
	if got := alertBannerText([]smart.Disk{{Kind: smart.KindATA, Attrs: []smart.Attr{{ID: 1}}}}, nil); !strings.Contains(got, "安全范围") {
		t.Fatalf("unexpected safe banner: %q", got)
	}
	if got := alertBannerText([]smart.Disk{{Kind: smart.KindATA}}, nil); !strings.Contains(got, "无法得出完整健康结论") {
		t.Fatalf("unexpected incomplete banner: %q", got)
	}
	vs := []health.Violation{{DiskIndex: 0, AttrName: "SMART_Overall_Health", Current: "FAILED", Limit: "PASSED", Severity: health.Critical}}
	if got := alertBannerText([]smart.Disk{{Kind: smart.KindATA}}, vs); !strings.Contains(got, "SMART_Overall_Health") || !strings.Contains(got, "未读取到 SMART 数据") {
		t.Fatalf("unexpected violation banner: %q", got)
	}
}

func TestReportBannerShowsStartupStatusWithoutDialog(t *testing.T) {
	text, color := reportBanner(nil, nil, "扫描失败：访问被拒绝")
	if !strings.Contains(text, "扫描失败：访问被拒绝") || !strings.Contains(text, "未发现支持 SMART") {
		t.Fatalf("unexpected startup banner: %q", text)
	}
	if color != walk.RGB(0xB0, 0x20, 0x20) {
		t.Fatalf("unexpected startup status color: %#v", color)
	}
}

func TestVirtualDiskIsNotReportedAsUnreadSMART(t *testing.T) {
	disks := []smart.Disk{{Kind: smart.KindUnknown, Model: "Virtual Disk"}}
	if unreadSMARTCount(disks) != 0 || smartApplicableCount(disks) != 0 {
		t.Fatalf("unexpected SMART applicability: unread=%d applicable=%d", unreadSMARTCount(disks), smartApplicableCount(disks))
	}
	if got := diskSummary(disks[0]); !strings.Contains(got, "SMART不适用") {
		t.Fatalf("unexpected virtual disk summary: %q", got)
	}
	if got := alertBannerText(disks, nil); !strings.Contains(got, "未发现支持 SMART") {
		t.Fatalf("unexpected virtual disk banner: %q", got)
	}
	if got := buildTextReport(disks, nil); !strings.Contains(got, "不提供 SMART 数据") || !strings.Contains(got, "未发现支持 SMART") {
		t.Fatalf("unexpected virtual disk report: %q", got)
	}
}

func TestReportModelEventsRemainStableAcrossTableModelReplacement(t *testing.T) {
	m := &reportModel{}
	if m.RowsReset() != m.RowsReset() || m.RowChanged() != m.RowChanged() || m.RowsChanged() != m.RowsChanged() || m.RowsInserted() != m.RowsInserted() || m.RowsRemoved() != m.RowsRemoved() {
		t.Fatal("report model must return stable event instances")
	}
}

func TestBuildExceptionReportContainsOnlyFeedbackRows(t *testing.T) {
	disks := []smart.Disk{{Index: 1, Model: "Test SSD\tModel", Attrs: []smart.Attr{{ID: 9, Name: "Power_On_Hours"}}}}
	vs := []health.Violation{{DiskIndex: 1, AttrName: "Command_Timeout", Current: "11", Limit: "≤ 10", Severity: health.Warning}}
	got := buildExceptionReport(disks, vs)
	if !strings.HasPrefix(got, "磁盘号\t型号\t异常项目\t当前值\t阈值\t级别\n") || !strings.Contains(got, "Disk1\tTest SSD Model\tCommand_Timeout\t11\t≤ 10\t警告\n") {
		t.Fatalf("unexpected exception report: %q", got)
	}
	if strings.Contains(got, "Power_On_Hours") {
		t.Fatalf("healthy attribute leaked into exception report: %q", got)
	}
	if got := buildExceptionReport(disks, nil); got != "未检测到 S.M.A.R.T 异常。\n" {
		t.Fatalf("unexpected clean exception report: %q", got)
	}
}

func TestSimulationFixtureLoadsExpectedGUIState(t *testing.T) {
	rw := &ReportWin{}
	if err := rw.setReportData(SimulatedFailureDisks()); err != nil {
		t.Fatalf("load simulated data: %v", err)
	}
	if len(rw.disks) != 2 || len(rw.violations) != 8 {
		t.Fatalf("unexpected simulated GUI state: disks=%d violations=%d", len(rw.disks), len(rw.violations))
	}
	if report := buildExceptionReport(rw.disks, rw.violations); !strings.Contains(report, "SIMULATED ATA FAILURE") || !strings.Contains(report, "SIMULATED NVME FAILURE") {
		t.Fatalf("simulation feedback report missing disk: %q", report)
	}
}
