package smart

import "testing"

func TestReadSMARTDataUsesStandardHeader(t *testing.T) {
	data := make([]byte, 512)
	data[0] = 1
	data[2] = 0x05
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
	if attrs[0].ID != 0x05 || attrs[0].Value != 0x64 || attrs[0].Raw != 0x000605040302 {
		t.Fatalf("unexpected attribute: %+v", attrs[0])
	}
}
