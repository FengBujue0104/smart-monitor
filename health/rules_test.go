package health

import (
	"math"
	"strings"
	"testing"

	"smonitor/smart"
)

func TestATACompositeRawDoesNotTriggerReadErrorWarning(t *testing.T) {
	d := smart.Disk{Index: 0, Kind: smart.KindATA, Attrs: []smart.Attr{{
		ID: 0x01, Raw: 123456, Value: 100, Thresh: 51,
	}}}
	if got := Evaluate([]smart.Disk{d}); len(got) != 0 {
		t.Fatalf("unexpected warning for healthy normalized read error rate: %+v", got)
	}
}

func TestATAReadErrorUsesNormalizedThreshold(t *testing.T) {
	d := smart.Disk{Index: 0, Kind: smart.KindATA, Attrs: []smart.Attr{{
		ID: 0x01, Raw: 123456, Value: 50, Thresh: 51,
	}}}
	got := Evaluate([]smart.Disk{d})
	if len(got) != 1 || got[0].Severity != Warning {
		t.Fatalf("expected normalized threshold warning: %+v", got)
	}
}

func TestATAAlternateTemperatureAttributesUseTemperatureThresholds(t *testing.T) {
	for _, id := range []int{0xB9, 0xBE, 0xC2} {
		d := smart.Disk{Index: 0, Kind: smart.KindATA, Attrs: []smart.Attr{{ID: id, Raw: 61}}}
		got := Evaluate([]smart.Disk{d})
		if len(got) != 1 || got[0].Severity != Critical || got[0].Current != "61°C" {
			t.Fatalf("attribute 0x%02X should create critical temperature violation: %+v", id, got)
		}
	}
}

func TestATAYMTCF3UsesTemperatureThresholdsOnlyForYMTC(t *testing.T) {
	ymtc := smart.Disk{Index: 0, Kind: smart.KindATA, Model: "ZHITAI TiPlus5000", Attrs: []smart.Attr{{ID: 0xF3, Raw: 61}}}
	got := Evaluate([]smart.Disk{ymtc})
	if len(got) != 1 || got[0].Severity != Critical || got[0].Current != "61°C" {
		t.Fatalf("YMTC F3 should create a critical temperature violation: %+v", got)
	}

	intel := smart.Disk{Index: 0, Kind: smart.KindATA, Model: "Intel SSD DC S3500", Attrs: []smart.Attr{{ID: 0xF3, Raw: 61}}}
	if got := Evaluate([]smart.Disk{intel}); len(got) != 0 {
		t.Fatalf("non-YMTC F3 must not be treated as temperature: %+v", got)
	}
}

func TestATACommandTimeoutWarnsOnlyAfterTenEvents(t *testing.T) {
	for _, test := range []struct {
		raw  uint64
		want int
	}{
		{raw: 1, want: 0},
		{raw: 10, want: 0},
		{raw: 11, want: 1},
	} {
		d := smart.Disk{Index: 0, Kind: smart.KindATA, Attrs: []smart.Attr{{ID: 0xBC, Raw: test.raw}}}
		got := Evaluate([]smart.Disk{d})
		if len(got) != test.want {
			t.Fatalf("Command_Timeout raw=%d: got %d violations, want %d (%+v)", test.raw, len(got), test.want, got)
		}
	}
}

func TestWDBlueSA510MediaWearoutUsesCrystalDiskInfoLifeRule(t *testing.T) {
	for _, test := range []struct {
		raw      uint64
		want     int
		severity Severity
		current  string
	}{
		{raw: 0x0200, want: 0}, // 98% remaining, as in the exported report.
		{raw: 0x5F00, want: 1, severity: Warning, current: "5% (remaining)"},
		{raw: 0x6400, want: 1, severity: Critical, current: "0% (remaining)"},
	} {
		d := smart.Disk{Index: 0, Kind: smart.KindATA, Model: "WD Blue SA510 2.5 500GB SSD", Attrs: []smart.Attr{{ID: 0xE6, Raw: test.raw}}}
		got := Evaluate([]smart.Disk{d})
		if len(got) != test.want {
			t.Fatalf("E6 raw=0x%X: got %d violations, want %d (%+v)", test.raw, len(got), test.want, got)
		}
		if test.want > 0 && (got[0].Severity != test.severity || got[0].Current != test.current) {
			t.Fatalf("E6 raw=0x%X: got %+v", test.raw, got[0])
		}
	}

	generic := smart.Disk{Index: 0, Kind: smart.KindATA, Model: "Generic SSD", Attrs: []smart.Attr{{ID: 0xE6, Raw: 0x6400}}}
	if got := Evaluate([]smart.Disk{generic}); len(got) != 0 {
		t.Fatalf("generic E6 must not use WD life rule: %+v", got)
	}
}

// TestCrystalDiskInfoExportedHealthyDisksRemainHealthy is a regression fixture
// transcribed from CrystalDiskInfo_20260718090515.txt. CrystalDiskInfo reports
// KIOXIA-EXCERIA at 96% and WD Blue SA510 at 98%, both in good condition.
func TestCrystalDiskInfoExportedHealthyDisksRemainHealthy(t *testing.T) {
	disks := []smart.Disk{
		{
			Index: 1, Kind: smart.KindATA, Model: "KIOXIA-EXCERIA SATA SSD",
			Attrs: []smart.Attr{
				{ID: 0x09, Raw: 30870, Value: 100, Thresh: 0},
				{ID: 0xA9, Raw: 0, Value: 100, Thresh: 10},
				{ID: 0xAD, Raw: 0, Value: 196, Thresh: 0},
				{ID: 0xC2, Raw: 0x002C00130022, Value: 66, Thresh: 20},
				{ID: 0xF1, Raw: 0x060F8A, Value: 100, Thresh: 0},
			},
		},
		{
			Index: 2, Kind: smart.KindATA, Model: "WD Blue SA510 2.5 500GB SSD",
			Attrs: []smart.Attr{
				{ID: 0x05, Raw: 0, Value: 100, Thresh: 10},
				{ID: 0xAD, Raw: 13, Value: 100, Thresh: 5},
				{ID: 0xB8, Raw: 0, Value: 100, Thresh: 97},
				{ID: 0xBB, Raw: 0, Value: 100, Thresh: 0},
				{ID: 0xBC, Raw: 1, Value: 100, Thresh: 0},
				{ID: 0xC2, Raw: 0x002800140023, Value: 100, Thresh: 14},
				{ID: 0xE6, Raw: 0x025000560250, Value: 100, Thresh: 0},
				{ID: 0xE8, Raw: 93, Value: 100, Thresh: 4},
				{ID: 0xE9, Raw: 5928, Value: 100, Thresh: 0},
				{ID: 0xF1, Raw: 12153, Value: 253, Thresh: 0},
				{ID: 0xF2, Raw: 13631, Value: 253, Thresh: 0},
			},
		},
	}
	if got := Evaluate(disks); len(got) != 0 {
		t.Fatalf("CrystalDiskInfo-good disks must not produce violations: %+v", got)
	}
}

func TestATAOverallSMARTFailureCreatesCriticalViolation(t *testing.T) {
	d := smart.Disk{Index: 0, Kind: smart.KindATA, SmartStatusKnown: true, SmartStatusPassed: false}
	got := Evaluate([]smart.Disk{d})
	if len(got) != 1 || got[0].AttrID != 0 || got[0].Severity != Critical || got[0].AttrName != "SMART_Overall_Health" {
		t.Fatalf("expected overall SMART failure violation: %+v", got)
	}
}

func TestATAUnknownOverallSMARTStatusDoesNotCreateViolation(t *testing.T) {
	d := smart.Disk{Index: 0, Kind: smart.KindATA, SmartStatusKnown: false, SmartStatusPassed: false}
	if got := Evaluate([]smart.Disk{d}); len(got) != 0 {
		t.Fatalf("unexpected violation for unknown overall status: %+v", got)
	}
}

func TestNVMeOverallSMARTFailureCreatesCriticalViolation(t *testing.T) {
	d := smart.Disk{Index: 0, Kind: smart.KindNVMe, SmartStatusKnown: true, SmartStatusPassed: false}
	got := Evaluate([]smart.Disk{d})
	if len(got) != 1 || got[0].AttrID != 0 || got[0].Severity != Critical || got[0].AttrName != "SMART_Overall_Health" {
		t.Fatalf("expected NVMe overall SMART failure violation: %+v", got)
	}
}

func TestInvalidATASMARTChecksumCreatesWarning(t *testing.T) {
	d := smart.Disk{Index: 0, Kind: smart.KindATA, SMARTChecksumKnown: true, SMARTChecksumValid: false}
	got := Evaluate([]smart.Disk{d})
	if len(got) != 1 || got[0].AttrID != -1 || got[0].Severity != Warning || got[0].AttrName != "SMART_Data_Checksum" {
		t.Fatalf("expected SMART checksum warning: %+v", got)
	}
}

func TestInvalidATASMARTChecksumSuppressesUntrustedAttributeViolations(t *testing.T) {
	d := smart.Disk{
		Index: 0, Kind: smart.KindATA, SMARTChecksumKnown: true, SMARTChecksumValid: false,
		Attrs: []smart.Attr{{ID: 0x05, Name: "Reallocated_Sector_Ct", Raw: 1}},
	}
	got := Evaluate([]smart.Disk{d})
	if len(got) != 1 || got[0].AttrID != -1 || got[0].Severity != Warning {
		t.Fatalf("corrupt SMART data should only produce integrity warning: %+v", got)
	}
}

func TestInvalidATASMARTThresholdChecksumCreatesWarning(t *testing.T) {
	d := smart.Disk{Index: 0, Kind: smart.KindATA, SMARTThresholdChecksumKnown: true, SMARTThresholdChecksumValid: false}
	got := Evaluate([]smart.Disk{d})
	if len(got) != 1 || got[0].AttrID != -2 || got[0].Severity != Warning || got[0].AttrName != "SMART_Threshold_Checksum" {
		t.Fatalf("expected SMART threshold checksum warning: %+v", got)
	}
}

func TestGenericATAThresholdUsesPreFailSeverity(t *testing.T) {
	tests := []struct {
		name  string
		flags uint16
		want  Severity
	}{
		{name: "pre-fail", flags: 0x0001, want: Critical},
		{name: "old-age", flags: 0x0002, want: Warning},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := smart.Disk{Index: 0, Kind: smart.KindATA, Attrs: []smart.Attr{{
				ID: 0xAA, Name: "Vendor_Health_Metric", Flags: tt.flags, Value: 10, Thresh: 10,
			}}}
			got := Evaluate([]smart.Disk{d})
			if len(got) != 1 || got[0].Severity != tt.want || got[0].Current != "10" || got[0].Limit != "> 10" {
				t.Fatalf("unexpected generic threshold violation: %+v", got)
			}
		})
	}
}

func TestDedicatedATAThresholdDoesNotCreateDuplicateGenericViolation(t *testing.T) {
	d := smart.Disk{Index: 0, Kind: smart.KindATA, Attrs: []smart.Attr{{
		ID: 0x01, Flags: 0x0001, Value: 10, Thresh: 10,
	}}}
	got := Evaluate([]smart.Disk{d})
	if len(got) != 1 || got[0].Severity != Warning {
		t.Fatalf("expected only dedicated read-error warning: %+v", got)
	}
}

func TestNVMePercentageUsedIsNotFailureAtFiftyPercent(t *testing.T) {
	for _, value := range []uint64{50, 79} {
		d := smart.Disk{Index: 0, Kind: smart.KindNVMe, Attrs: []smart.Attr{{
			ID: smart.NVMePercentUsed, Raw: value,
		}}}
		if got := Evaluate([]smart.Disk{d}); len(got) != 0 {
			t.Fatalf("unexpected warning at %d%% used: %+v", value, got)
		}
	}
}

func TestNVMeSpareUsesDeviceThreshold(t *testing.T) {
	d := smart.Disk{Index: 0, Kind: smart.KindNVMe, Attrs: []smart.Attr{
		{ID: smart.NVMeAvailableSpare, Raw: 8},
		{ID: smart.NVMeAvailSpareThresh, Raw: 10},
	}}
	got := Evaluate([]smart.Disk{d})
	if len(got) != 1 || got[0].Severity != Critical {
		t.Fatalf("expected spare threshold failure: %+v", got)
	}
}

func TestNVMeTemperatureSensorOverheatCreatesViolation(t *testing.T) {
	d := smart.Disk{Index: 0, Kind: smart.KindNVMe, Attrs: []smart.Attr{{
		ID: smart.NVMeTemperatureSensor1, Name: "Temperature_Sensor_1_Kelvin", Raw: 334,
	}}}
	got := Evaluate([]smart.Disk{d})
	if len(got) != 1 || got[0].Severity != Critical || got[0].Current != "61°C" {
		t.Fatalf("expected critical sensor temperature violation: %+v", got)
	}
}

func TestNVMeCriticalWarningIsFormattedAsHex(t *testing.T) {
	d := smart.Disk{Index: 0, Kind: smart.KindNVMe, Attrs: []smart.Attr{{
		ID: smart.NVMeCriticalWarning, Raw: 0x10,
	}}}
	got := Evaluate([]smart.Disk{d})
	if len(got) != 1 || got[0].Current != "0x10 (易失性缓存备份设备故障)" {
		t.Fatalf("unexpected critical warning: %+v", got)
	}
}

func TestNVMeCriticalWarningTextExplainsMixedAndUnknownBits(t *testing.T) {
	got := nvmeCriticalWarningText(0x8B)
	want := "0x8b (可用备用空间低于阈值；温度超过临界阈值；控制器已进入只读模式；未知保留位 0x80)"
	if got != want {
		t.Fatalf("critical warning text = %q, want %q", got, want)
	}
}

func TestNVMeEnduranceGroupCriticalWarningCreatesViolation(t *testing.T) {
	d := smart.Disk{Index: 0, Kind: smart.KindNVMe, Attrs: []smart.Attr{{
		ID: smart.NVMeEnduranceGroupCriticalWarning, Raw: 0x04,
	}}}
	got := Evaluate([]smart.Disk{d})
	if len(got) != 1 || got[0].Severity != Critical || !strings.Contains(got[0].Current, "可靠性已降级") {
		t.Fatalf("unexpected endurance-group warning: %+v", got)
	}
}

func TestNVMeCountersDoNotOverflowWhenReported(t *testing.T) {
	d := smart.Disk{Index: 0, Kind: smart.KindNVMe, Attrs: []smart.Attr{{
		ID: smart.NVMeMediaErrors, Raw: math.MaxUint64,
	}}}
	got := Evaluate([]smart.Disk{d})
	if len(got) != 1 || got[0].Current != "18446744073709551615" {
		t.Fatalf("unexpected media error count: %+v", got)
	}
}

func TestNVMeMediaErrorsWithNonZeroHighHalfAreCritical(t *testing.T) {
	d := smart.Disk{Index: 0, Kind: smart.KindNVMe, Attrs: []smart.Attr{{
		ID: smart.NVMeMediaErrors, Raw: 0, RawHigh: 1,
	}}}
	got := Evaluate([]smart.Disk{d})
	if len(got) != 1 || got[0].Current != "0x00000000000000010000000000000000" {
		t.Fatalf("unexpected 128-bit media error violation: %+v", got)
	}
}
