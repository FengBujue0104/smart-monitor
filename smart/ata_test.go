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

func TestATAAttrNameForModelCrystalDiskInfoAliases(t *testing.T) {
	if got := ATAAttrNameForModel("KIOXIA-EXCERIA SATA SSD", 0xA8); got != "SATA_PHY_Error_Count" {
		t.Fatalf("KIOXIA A8 alias = %q", got)
	}
	if got := ATAAttrNameForModel("KIOXIA-EXCERIA SATA SSD", 0xF1); got != "Host_Writes" {
		t.Fatalf("KIOXIA F1 alias = %q", got)
	}
	if got := ATAAttrNameForModel("WD Blue SA510 2.5 500GB SSD", 0xAC); got != "Erase_Fail_Count" {
		t.Fatalf("WD Blue AC alias = %q", got)
	}
	if got := ATAAttrNameForModel("WD Blue SA510 2.5 500GB SSD", 0xF2); got != "Host_Reads_GB" {
		t.Fatalf("WD Blue F2 alias = %q", got)
	}
	if got := ATAAttrNameForModel("ZHITAI TiPlus5000", 0xF3); got != "Temperature_Celsius" {
		t.Fatalf("ZHITAI F3 alias = %q", got)
	}
	if got := ATAAttrNameForModel("ZHITAI TiPlus5000", 0xF1); got != "Host_Writes" {
		t.Fatalf("ZHITAI F1 alias = %q", got)
	}
	if got := ATAAttrNameForModel("Samsung SSD 870 EVO 1TB", 0xF2); got != "Host_Reads" {
		t.Fatalf("Samsung F2 alias = %q", got)
	}
	if got := ATAAttrNameForModel("CT1000MX500SSD1", 0xF1); got != "Host_Writes" {
		t.Fatalf("Crucial MX500 F1 alias = %q", got)
	}
	if got := ATAAttrNameForModel("INTEL SSDSC2BA200G3", 0xF3); got != "NAND_Writes" {
		t.Fatalf("Intel F3 alias = %q", got)
	}
	if got := ATAAttrNameForModel("Seagate ZA240CV10001", 0xEA); got != "NAND_Writes_SLC" {
		t.Fatalf("Seagate EA alias = %q", got)
	}
	if got := ATAAttrNameForModel("KINGSTON SKC600512G", 0xF2); got != "Host_Reads" {
		t.Fatalf("Kingston KC600 F2 alias = %q", got)
	}
	if got := ATAAttrNameForModel("TOSHIBA THNSNC128GCSJ", 0xF1); got != "Host_Writes" {
		t.Fatalf("Toshiba F1 alias = %q", got)
	}
	if got := ATAAttrNameForModel("Intel SSD DC S3500", 0xF3); got == "Temperature_Celsius" {
		t.Fatalf("Intel F3 must not be interpreted as temperature")
	}
}

func TestATATemperatureAttributeForModelLimitsF3ToYMTC(t *testing.T) {
	for _, test := range []struct {
		model string
		id    int
		want  bool
	}{
		{"ZHITAI TiPlus5000", 0xF3, true},
		{"致态 TiPlus5000", 0xF3, true},
		{"Intel SSD DC S3500", 0xF3, false},
		{"Any SATA SSD", 0xC2, true},
	} {
		if got := ATATemperatureAttributeForModel(test.model, test.id); got != test.want {
			t.Fatalf("temperature attribute (%q, 0x%02X) = %v, want %v", test.model, test.id, got, test.want)
		}
	}
	if !IsYMTCSATAModel("ZHITAI TiPlus5000") || !IsYMTCSATAModel("致态 TiPlus5000") || IsYMTCSATAModel("Intel SSD DC S3500") {
		t.Fatal("unexpected YMTC model identification")
	}
	if !IsCrucial32MBHostCounterModel("CT1000MX500SSD1") || !IsCrucial32MBHostCounterModel("Crucial BX500SSD") || IsCrucial32MBHostCounterModel("Micron M600") {
		t.Fatal("unexpected Crucial 32 MB model identification")
	}
	if !IsIntelOrSolidigmSATAModel("INTEL SSDSC2BA200G3") || !IsIntelOrSolidigmSATAModel("Solidigm D3-S4510") || IsIntelOrSolidigmSATAModel("KIOXIA SATA SSD") {
		t.Fatal("unexpected Intel/Solidigm model identification")
	}
	if !IsKingstonKC600Model("KINGSTON SKC600512G") || IsKingstonKC600Model("KINGSTON SA400S37") {
		t.Fatal("unexpected Kingston KC600 model identification")
	}
	if !IsToshiba32MBHostCounterModel("TOSHIBA THNSNC128GCSJ") || !IsToshiba32MBHostCounterModel("TOSHIBA TR200") || IsToshiba32MBHostCounterModel("TOSHIBA Generic SATA") {
		t.Fatal("unexpected Toshiba 32 MB model identification")
	}
}

func TestATAHealthPercentForVerifiedCrystalDiskInfoModels(t *testing.T) {
	kioxia := []Attr{{ID: 0xAD, Value: 196}}
	if got, ok := ATAHealthPercentForModel("KIOXIA-EXCERIA SATA SSD", kioxia); !ok || got != 96 {
		t.Fatalf("KIOXIA health = %d, %v; want 96, true", got, ok)
	}
	wd := []Attr{{ID: 0xE6, Raw: 0x025000560250}}
	if got, ok := ATAHealthPercentForModel("WD Blue SA510 2.5 500GB SSD", wd); !ok || got != 98 {
		t.Fatalf("WD Blue SA510 health = %d, %v; want 98, true", got, ok)
	}
	if _, ok := ATAHealthPercentForModel("Generic SSD", wd); ok {
		t.Fatal("generic E6 must not be assigned a vendor health percentage")
	}
	samsung := []Attr{{ID: 0xB1, Value: 97}}
	if got, ok := ATAHealthPercentForModel("Samsung SSD 870 EVO 1TB", samsung); !ok || got != 97 {
		t.Fatalf("Samsung B1 health = %d, %v; want 97, true", got, ok)
	}
	if _, ok := ATAHealthPercentForModel("SAMSUNG HD502HJ", samsung); ok {
		t.Fatal("Samsung HDD B1 must not use the SSD health rule")
	}
}

func TestATACounterUnitForModelUsesVerifiedMappings(t *testing.T) {
	for _, test := range []struct {
		model string
		id    int
		unit  ATACounterUnit
		ok    bool
	}{
		{"KIOXIA-EXCERIA SATA SSD", 0xF1, ATACounterUnit32MB, true},
		{"WD Blue SA510 2.5 500GB SSD", 0xE9, ATACounterUnitGB, true},
		{"ZHITAI TiPlus5000", 0xF2, ATACounterUnit512B, true},
		{"Samsung SSD 870 EVO", 0xF1, ATACounterUnit512B, true},
		{"CT1000MX500SSD1", 0xF2, ATACounterUnit32MB, true},
		{"INTEL SSDSC2BA200G3", 0xF3, ATACounterUnit32MB, true},
		{"KINGSTON SKC600512G", 0xF1, ATACounterUnit32MB, true},
		{"TOSHIBA THNSNC128GCSJ", 0xF2, ATACounterUnit32MB, true},
		{"Seagate ZA240CV10001", 0xEA, ATACounterUnitGB, true},
		{"Generic SATA SSD", 0xF1, ATACounterUnitUnknown, false},
	} {
		unit, ok := ATACounterUnitForModel(test.model, test.id)
		if unit != test.unit || ok != test.ok {
			t.Fatalf("counter unit (%q, 0x%02X) = %v, %v; want %v, %v", test.model, test.id, unit, ok, test.unit, test.ok)
		}
	}
}

func TestATAVendorProfilesUseSpecificMatchBeforeGenericMatch(t *testing.T) {
	for _, test := range []struct {
		model string
		want  string
	}{
		{"WD Blue SA510 2.5 500GB SSD", "WD Blue SA510"},
		{"WD Blue 3D NAND SATA SSD", "WD Blue SATA"},
		{"KINGSTON SKC600512G", "Kingston KC600"},
		{"Generic SATA SSD", ""},
	} {
		profile := ataVendorProfileForModel(test.model)
		if test.want == "" {
			if profile != nil {
				t.Fatalf("profile for %q = %q, want none", test.model, profile.name)
			}
			continue
		}
		if profile == nil || profile.name != test.want {
			got := ""
			if profile != nil {
				got = profile.name
			}
			t.Fatalf("profile for %q = %q, want %q", test.model, got, test.want)
		}
	}
}

func TestSKHynixProfilesUseSourceDefinedUnitsAndLife(t *testing.T) {
	for _, test := range []struct {
		model string
		attr  Attr
		unit  ATACounterUnit
		life  int
	}{
		{"HFS128G32TND-3110A", Attr{ID: 0xE9, Raw: 95}, ATACounterUnitGB, 5},
		{"HFS128G32TNF-N3A0A", Attr{ID: 0xE9, Raw: 5}, ATACounterUnitGB, 5},
		{"SK hynix SC311 SATA 256GB", Attr{ID: 0xE9, Raw: 5}, ATACounterUnit512B, 5},
		{"SK hynix HFS512GDE9X073N", Attr{ID: 0xE9, Value: 90}, ATACounterUnitGB, 90},
	} {
		if got, ok := ATAHealthPercentForModel(test.model, []Attr{test.attr}); !ok || got != test.life {
			t.Fatalf("SK hynix life for %q = %d, %v; want %d, true", test.model, got, ok, test.life)
		}
		if got, ok := ATACounterUnitForModel(test.model, 0xF1); !ok || got != test.unit {
			t.Fatalf("SK hynix unit for %q = %v, %v; want %v, true", test.model, got, ok, test.unit)
		}
	}
}

func TestSanDiskProfilesUseSourceDefinedUnitsAndLife(t *testing.T) {
	for _, test := range []struct {
		model string
		unit  ATACounterUnit
	}{
		{"SanDisk X400 2.5 7MM 256GB", ATACounterUnit512B},
		{"SanDisk SSD PLUS 480GB", ATACounterUnitGB},
	} {
		if got, ok := ATACounterUnitForModel(test.model, 0xF1); !ok || got != test.unit {
			t.Fatalf("SanDisk counter unit for %q = %v, %v; want %v, true", test.model, got, ok, test.unit)
		}
		if got, ok := ATAHealthPercentForModel(test.model, []Attr{{ID: 0xE6, Raw: 0x5F00}}); !ok || got != 5 {
			t.Fatalf("SanDisk life for %q = %d, %v; want 5, true", test.model, got, ok)
		}
	}
	if _, ok := ATACounterUnitForModel("SanDisk SSD P4 USB Memory", 0xF1); ok {
		t.Fatal("SanDisk USB P4 must not use SATA profile")
	}
}
