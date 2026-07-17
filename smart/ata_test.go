package smart

import "testing"

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
