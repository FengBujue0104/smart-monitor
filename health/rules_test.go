package health

import (
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
