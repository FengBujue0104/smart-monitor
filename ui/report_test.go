package ui

import (
	"strings"
	"testing"

	"smonitor/smart"
)

func TestAttrDisplaySeparatesRawAndNormalizedValues(t *testing.T) {
	ataTemp := smart.Attr{ID: 0xC2, Raw: 42, Value: 100, Worst: 90, Kind: "ata"}
	if got := attrRawStr(ataTemp); got != "42°C (raw 42)" {
		t.Fatalf("unexpected ATA raw display: %q", got)
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

	nvmeWrite := smart.Attr{ID: smart.NVMeDataUnitsWritten, Raw: 1000000, Kind: "nvme"}
	if got := attrRawStr(nvmeWrite); !strings.Contains(got, "0.51 TB") || !strings.Contains(got, "1000000 units") {
		t.Fatalf("unexpected NVMe data unit display: %q", got)
	}
}
