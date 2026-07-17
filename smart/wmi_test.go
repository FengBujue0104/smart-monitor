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

func TestFindDataForDriveNormalizesModelSeparators(t *testing.T) {
	m := map[string][]byte{
		"SCSI\\Disk&Ven_Samsung&Prod_SSD_980_PRO\\1": make([]byte, 14),
	}
	drv := wmiDiskDrive{Model: "Samsung SSD 980 PRO"}
	data, instance := findDataForDrive(m, drv)
	if data == nil || instance == "" {
		t.Fatalf("expected normalized model match, instance=%q", instance)
	}
}

func TestFindStatusForDriveUsesUniqueDeviceMatch(t *testing.T) {
	m := map[string]bool{
		"SCSI\\Disk&Ven_A&Prod_DriveA\\1": true,
	}
	drv := wmiDiskDrive{Model: "DriveA", SerialNumber: "serial-a"}
	predictFailure, ok := findStatusForDrive(m, drv)
	if !ok || !predictFailure {
		t.Fatalf("expected matching failure status, got status=%v ok=%v", predictFailure, ok)
	}
}

func TestFindStatusForDriveRejectsAmbiguousModel(t *testing.T) {
	m := map[string]bool{
		"SCSI\\Disk&Ven_A&Prod_Same\\1": true,
		"SCSI\\Disk&Ven_A&Prod_Same\\2": false,
	}
	if _, ok := findStatusForDrive(m, wmiDiskDrive{Model: "Same"}); ok {
		t.Fatal("ambiguous model should not produce a status")
	}
}

func TestApplyWMIATADataChecksSMARTChecksum(t *testing.T) {
	data := make([]byte, 512)
	data[0] = 1
	data[2] = 0x05
	data[3] = 0x01
	data[5] = 100
	for _, b := range data[:511] {
		data[511] -= b
	}

	d := Disk{Model: "Test"}
	applyWMIATAData(&d, data, nil)
	if len(d.Attrs) != 1 || !d.SMARTChecksumKnown || !d.SMARTChecksumValid {
		t.Fatalf("unexpected valid WMI SMART data: %+v", d)
	}

	data[10]++
	applyWMIATAData(&d, data, nil)
	if d.SMARTChecksumValid {
		t.Fatalf("expected invalid WMI SMART checksum: %+v", d)
	}
}

func TestClassifyWMIDiskDetectsNVMeWithoutSMARTDataInstance(t *testing.T) {
	drv := wmiDiskDrive{PNPDeviceID: `SCSI\Disk&Ven_NVMe&Prod_SSD\1`}
	if got := classifyWMIDisk(drv, ""); got != KindNVMe {
		t.Fatalf("WMI disk kind = %q, want NVMe", got)
	}
}
