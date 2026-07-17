package smart

import (
	"encoding/binary"
	"testing"
)

func TestParseStorageDescriptorTrimsIdentityFields(t *testing.T) {
	desc := make([]byte, 128)
	binary.LittleEndian.PutUint32(desc[0x1C:0x20], 0x11)
	writeString := func(fieldOffset, valueOffset uint32, value string) {
		binary.LittleEndian.PutUint32(desc[fieldOffset:fieldOffset+4], valueOffset)
		copy(desc[valueOffset:], value)
	}
	writeString(0x0C, 0x30, "  Vendor  ")
	writeString(0x10, 0x40, "  Fast SSD  ")
	writeString(0x14, 0x50, "  1.23  ")
	writeString(0x18, 0x60, "  SN-42  ")

	vendor, product, revision, serial, busType := parseStorageDescriptor(desc)
	if vendor != "Vendor" || product != "Fast SSD" || revision != "1.23" || serial != "SN-42" || busType != 0x11 {
		t.Fatalf("unexpected descriptor: vendor=%q product=%q revision=%q serial=%q bus=%#x", vendor, product, revision, serial, busType)
	}
}

func TestStorageBusSupportsSMARTExcludesVirtualDisks(t *testing.T) {
	for _, busType := range []uint32{storageBusTypeVirtual, storageBusTypeFileBackedVirt, storageBusTypeSpaces} {
		if storageBusSupportsSMART(busType) {
			t.Fatalf("virtual bus type %#x should not be probed for SMART", busType)
		}
	}
	if !storageBusSupportsSMART(0x0B) { // SATA
		t.Fatal("SATA should remain eligible for SMART probing")
	}
}
