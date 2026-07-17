package smart

import (
	"encoding/binary"
	"testing"
)

func TestBuildSMARTReturnStatus(t *testing.T) {
	buf := buildSMARTReturnStatus()
	if len(buf) != 48 || binary.LittleEndian.Uint16(buf[0:2]) != 48 {
		t.Fatalf("unexpected ATA pass-through buffer length/header: %d", len(buf))
	}
	if buf[40] != SMART_RETURN_STATUS || buf[42] != 0 || buf[43] != 0x4F || buf[44] != 0xC2 || buf[46] != ATA_CMD_SMART {
		t.Fatalf("unexpected task file: %x", buf[40:48])
	}
}

func TestParseSMARTReturnStatus(t *testing.T) {
	passed := make([]byte, 8)
	passed[3], passed[4] = 0x4F, 0xC2
	if ok, err := parseSMARTReturnStatus(passed); err != nil || !ok {
		t.Fatalf("expected SMART pass, got ok=%v err=%v", ok, err)
	}
	fail := make([]byte, 8)
	fail[3], fail[4] = 0xF4, 0x2C
	if ok, err := parseSMARTReturnStatus(fail); err != nil || ok {
		t.Fatalf("expected SMART failure, got ok=%v err=%v", ok, err)
	}
}

func TestSMARTStatusTaskFileRejectsTruncatedResponse(t *testing.T) {
	buf := make([]byte, 48)
	if _, err := smartStatusTaskFile(buf, 40, 40); err == nil {
		t.Fatal("expected short response error")
	}
	if taskFile, err := smartStatusTaskFile(buf, 48, 40); err != nil || len(taskFile) != 8 {
		t.Fatalf("expected complete task file, got %x err=%v", taskFile, err)
	}
}
