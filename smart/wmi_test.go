package smart

import "testing"

func TestApplyThresholdsKeepsPhysicalSlots(t *testing.T) {
	attrs := []Attr{{ID: 0x05}, {ID: 0xC5}}
	thresholds := make([]byte, 2+30*12)
	thresholds[2] = 0x05
	thresholds[3] = 36
	thresholds[2+2*12] = 0xC5
	thresholds[2+2*12+1] = 18

	applyThresholds(attrs, thresholds)
	if attrs[0].Thresh != 36 || attrs[1].Thresh != 18 {
		t.Fatalf("unexpected thresholds: %+v", attrs)
	}
}

func TestFindDataForDriveDoesNotReuseUnmatchedData(t *testing.T) {
	m := map[string][]byte{
		"SCSI\\Disk&Ven_A&Prod_DriveA\\1": make([]byte, 14),
	}
	drv := wmiDiskDrive{Model: "DriveB", SerialNumber: "serial-b"}
	data, instance := findDataForDrive(m, drv)
	if data != nil || instance != "" {
		t.Fatalf("unexpected fallback match: data=%v instance=%q", data, instance)
	}
}
