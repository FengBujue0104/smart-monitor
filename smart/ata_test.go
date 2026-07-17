package smart

import "testing"

func TestBuildSmartCmdUsesATASMARTTaskFileSignature(t *testing.T) {
	cmd := buildSmartCmd(ATA_CMD_SMART, SMART_READ_DATA, 3, make([]byte, 512))
	regs := cmd[0x04:0x0C]
	if regs[0] != SMART_READ_DATA || regs[1] != 1 || regs[2] != 0 || regs[3] != 0x4F || regs[4] != 0xC2 || regs[6] != ATA_CMD_SMART {
		t.Fatalf("unexpected SMART task file: %x", regs)
	}
	if cmd[0x0C] != 3 {
		t.Fatalf("unexpected drive number: %d", cmd[0x0C])
	}
}

func TestReadSMARTDataUsesStandardHeader(t *testing.T) {
	data := make([]byte, 512)
	data[0] = 1
	data[2] = 0x05
	data[3] = 0x35
	data[5] = 0x64
	data[6] = 1
	data[7] = 2
	data[8] = 3
	data[9] = 4
	data[10] = 5
	data[11] = 6

	attrs := parseSMARTData(data)
	if len(attrs) != 1 {
		t.Fatalf("got %d attributes, want 1", len(attrs))
	}
	if attrs[0].ID != 0x05 || attrs[0].Flags != 0x35 || attrs[0].Value != 0x64 || attrs[0].Raw != 0x000605040302 {
		t.Fatalf("unexpected attribute: %+v", attrs[0])
	}
}

func TestReadSMARTDataKeepsStandardHeaderWhenFirstSlotIsEmpty(t *testing.T) {
	data := make([]byte, 512)
	data[0] = 1
	off := 2 + 12
	data[off] = 0xC2
	data[off+3] = 100
	data[off+4] = 90
	data[off+5] = 42

	attrs := parseSMARTData(data)
	if len(attrs) != 1 || attrs[0].ID != 0xC2 || attrs[0].Raw != 42 {
		t.Fatalf("unexpected attributes with empty first slot: %+v", attrs)
	}
}

func TestParseSMARTDriverStatus(t *testing.T) {
	out := make([]byte, 16)
	if err := parseSMARTDriverStatus(out); err != nil {
		t.Fatalf("unexpected successful status error: %v", err)
	}
	out[4] = 1
	if err := parseSMARTDriverStatus(out); err == nil {
		t.Fatal("expected driver error")
	}
	out[4], out[5] = 0, 0x51
	if err := parseSMARTDriverStatus(out); err == nil {
		t.Fatal("expected IDE error")
	}
}

func TestSMARTChecksumValid(t *testing.T) {
	data := make([]byte, 512)
	data[0] = 1
	data[511] = 0xFF
	var sum byte
	for _, b := range data[:511] {
		sum += b
	}
	data[511] = -sum
	if !smartChecksumValid(data) {
		t.Fatal("expected valid SMART checksum")
	}
	data[10]++
	if smartChecksumValid(data) {
		t.Fatal("expected invalid SMART checksum")
	}
}

func TestParseSMARTThresholdsSupportsHeaderlessResponse(t *testing.T) {
	data := make([]byte, 512)
	data[0] = 0x05
	data[1] = 36
	off := 12
	data[off] = 0xC5
	data[off+1] = 18

	thresholds := parseSMARTThresholds(data)
	if thresholds[0x05] != 36 || thresholds[0xC5] != 18 {
		t.Fatalf("unexpected headerless thresholds: %+v", thresholds)
	}
}
