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

func TestBuildNVMeIdentifyControllerUsesAdminCommandLayout(t *testing.T) {
	buf := buildNVMeAdminCommand(NVMeIdentify, nvmeIdentifyControllerBytes, NVMeIdentifyController)
	if len(buf) != 144+nvmeIdentifyControllerBytes {
		t.Fatalf("identify buffer length = %d", len(buf))
	}
	if buf[80] != NVMeIdentify || binary.LittleEndian.Uint32(buf[120:124]) != NVMeIdentifyController {
		t.Fatalf("unexpected identify command: opcode=0x%02X cdw10=0x%08X", buf[80], binary.LittleEndian.Uint32(buf[120:124]))
	}
	if binary.LittleEndian.Uint32(buf[0x24:0x28]) != nvmeIdentifyControllerBytes {
		t.Fatalf("identify data length = %d", binary.LittleEndian.Uint32(buf[0x24:0x28]))
	}
}

func TestBuildNVMeHealthLogPropertyQueryUsesWindowsSDKLayout(t *testing.T) {
	buf := buildNVMeHealthLogPropertyQuery()
	if len(buf) != 48 {
		t.Fatalf("property query length = %d, want 48", len(buf))
	}
	if binary.LittleEndian.Uint32(buf[0:4]) != storageDeviceProtocolQuery ||
		binary.LittleEndian.Uint32(buf[4:8]) != PropertyStandardQuery {
		t.Fatalf("unexpected property-query header: % X", buf[:8])
	}
	protocol := buf[8:]
	if binary.LittleEndian.Uint32(protocol[0:4]) != storageProtocolTypeNVMe ||
		binary.LittleEndian.Uint32(protocol[4:8]) != nvmeDataTypeLogPage ||
		binary.LittleEndian.Uint32(protocol[8:12]) != NVMeLogID_SMART_Health ||
		binary.LittleEndian.Uint32(protocol[16:20]) != 40 ||
		binary.LittleEndian.Uint32(protocol[20:24]) != 512 {
		t.Fatalf("unexpected NVMe property query: % X", protocol)
	}
}

func TestParseNVMeHealthLogPropertyResponseUsesReturnedDataOffset(t *testing.T) {
	buf := make([]byte, 48+512)
	protocol := buf[8:48]
	binary.LittleEndian.PutUint32(protocol[0:4], storageProtocolTypeNVMe)
	binary.LittleEndian.PutUint32(protocol[4:8], nvmeDataTypeLogPage)
	binary.LittleEndian.PutUint32(protocol[16:20], 40)
	binary.LittleEndian.PutUint32(protocol[20:24], 512)
	buf[48] = 0x08
	data, err := parseNVMeHealthLogPropertyResponse(buf, uint32(len(buf)))
	if err != nil || len(data) != 512 || data[0] != 0x08 {
		t.Fatalf("property response parse = % X, %v", data, err)
	}
	binary.LittleEndian.PutUint32(protocol[16:20], 39)
	if _, err := parseNVMeHealthLogPropertyResponse(buf, uint32(len(buf))); err == nil {
		t.Fatal("invalid response offset should fail")
	}
}

func TestParseNVMeCompositeTemperatureThresholds(t *testing.T) {
	identify := make([]byte, nvmeIdentifyControllerBytes)
	binary.LittleEndian.PutUint16(identify[266:268], 333)
	binary.LittleEndian.PutUint16(identify[268:270], 343)
	warningK, criticalK := parseNVMeCompositeTemperatureThresholds(identify)
	if warningK != 333 || criticalK != 343 {
		t.Fatalf("thresholds = %dK/%dK, want 333K/343K", warningK, criticalK)
	}
	if warningK, criticalK := parseNVMeCompositeTemperatureThresholds(make([]byte, 269)); warningK != 0 || criticalK != 0 {
		t.Fatalf("short identify data produced thresholds: %dK/%dK", warningK, criticalK)
	}
}

func TestParseNVMeHealthLog(t *testing.T) {
	data := make([]byte, 512)
	data[0] = 0x08
	binary.LittleEndian.PutUint16(data[1:3], 300)
	data[3] = 95
	data[4] = 10
	data[5] = 7
	data[6] = 0x04
	binary.LittleEndian.PutUint64(data[0x20:0x28], 1000000)
	binary.LittleEndian.PutUint64(data[0x28:0x30], 1)
	binary.LittleEndian.PutUint64(data[0x70:0x78], 12)
	binary.LittleEndian.PutUint64(data[0x80:0x88], 34)
	binary.LittleEndian.PutUint64(data[0xA0:0xA8], 2)
	binary.LittleEndian.PutUint32(data[0xC0:0xC4], 9)
	binary.LittleEndian.PutUint16(data[0xC8:0xCA], 310)

	attrs := parseNVMeHealthLog(data)
	values := map[int]uint64{}
	for _, a := range attrs {
		values[a.ID] = a.Raw
	}
	if values[NVMeTemperature] != 300 || values[NVMeTemperatureSensor1] != 310 || values[NVMePowerCycles] != 12 ||
		values[NVMePowerOnHours] != 34 || values[NVMeMediaErrors] != 2 ||
		values[NVMeWarningTempTime] != 9 || values[NVMeReadOnly] != 1 || values[NVMeEnduranceGroupCriticalWarning] != 0x04 {
		t.Fatalf("unexpected NVMe attributes: %+v", values)
	}
	for _, a := range attrs {
		if a.ID == NVMeDataUnitsRead && (a.Raw != 1000000 || a.RawHigh != 1) {
			t.Fatalf("unexpected 128-bit data units: %+v", a)
		}
	}
}

func TestParseNVMeHealthLogAllowsPartialTemperatureSensorArray(t *testing.T) {
	data := make([]byte, 0xCA) // 基础字段 + 仅第一个可选温度传感器
	binary.LittleEndian.PutUint16(data[1:3], 300)
	binary.LittleEndian.PutUint16(data[0xC8:0xCA], 310)

	attrs := parseNVMeHealthLog(data)
	values := map[int]uint64{}
	for _, a := range attrs {
		values[a.ID] = a.Raw
	}
	if values[NVMeTemperature] != 300 || values[NVMeTemperatureSensor1] != 310 {
		t.Fatalf("unexpected attributes from partial sensor array: %+v", values)
	}
	if _, ok := values[NVMeTemperatureSensor2]; ok {
		t.Fatalf("unexpected second sensor from truncated data: %+v", values)
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
