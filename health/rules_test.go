package health

import (
	"math"
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

func TestNVMeCriticalWarningIsFormattedAsHex(t *testing.T) {
	d := smart.Disk{Index: 0, Kind: smart.KindNVMe, Attrs: []smart.Attr{{
		ID: smart.NVMeCriticalWarning, Raw: 0x10,
	}}}
	got := Evaluate([]smart.Disk{d})
	if len(got) != 1 || got[0].Current != "0x10" {
		t.Fatalf("unexpected critical warning: %+v", got)
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
