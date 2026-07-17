package smart

import (
	"encoding/binary"
	"testing"
)

func TestBuildNVMeGetLogPageUsesProtocolCommandLayout(t *testing.T) {
	buf := buildNVMeGetLogPage(NVMeLogID_SMART_Health, 512)
	if len(buf) != 656 {
		t.Fatalf("got buffer length %d, want 656", len(buf))
	}
	if binary.LittleEndian.Uint32(buf[0x00:0x04]) != 1 ||
		binary.LittleEndian.Uint32(buf[0x04:0x08]) != 80 ||
		binary.LittleEndian.Uint32(buf[0x08:0x0C]) != STORAGE_PROTOCOL_TYPE_NVMe {
		t.Fatalf("invalid protocol header: %x", buf[:16])
	}
	if buf[80] != NVMeGetLogPage {
		t.Fatalf("got opcode 0x%02x, want 0x02", buf[80])
	}
	if got := binary.LittleEndian.Uint32(buf[120:124]); got != (127<<16)|NVMeLogID_SMART_Health {
		t.Fatalf("got CDW10 0x%08x", got)
	}
	if binary.LittleEndian.Uint32(buf[0x34:0x38]) != 144 {
		t.Fatalf("invalid data offset")
	}
}

func TestParseNVMeHealthLog(t *testing.T) {
	data := make([]byte, 512)
	data[0] = 0x08
	binary.LittleEndian.PutUint16(data[1:3], 300)
	data[3] = 95
	data[4] = 10
	data[5] = 7
	binary.LittleEndian.PutUint64(data[0x30:0x38], 12)
	binary.LittleEndian.PutUint64(data[0x38:0x40], 34)
	binary.LittleEndian.PutUint64(data[0x48:0x50], 2)
	binary.LittleEndian.PutUint32(data[0x58:0x5C], 9)
	binary.LittleEndian.PutUint16(data[0x60:0x62], 310)

	attrs := parseNVMeHealthLog(data)
	values := map[int]uint64{}
	for _, a := range attrs {
		values[a.ID] = a.Raw
	}
	if values[NVMeTemperature] != 300 || values[NVMeTemperatureSensor1] != 310 || values[NVMePowerCycles] != 12 ||
		values[NVMePowerOnHours] != 34 || values[NVMeMediaErrors] != 2 ||
		values[NVMeWarningTempTime] != 9 || values[NVMeReadOnly] != 1 {
		t.Fatalf("unexpected NVMe attributes: %+v", values)
	}
}

func TestParseNVMeProtocolStatus(t *testing.T) {
	buf := make([]byte, 0x18)
	binary.LittleEndian.PutUint32(buf[0x10:0x14], 1)
	if err := parseNVMeProtocolStatus(buf); err != nil {
		t.Fatalf("unexpected success error: %v", err)
	}
	binary.LittleEndian.PutUint32(buf[0x10:0x14], 2)
	binary.LittleEndian.PutUint32(buf[0x14:0x18], 0x1234)
	if err := parseNVMeProtocolStatus(buf); err == nil {
		t.Fatal("expected protocol failure")
	}
}
