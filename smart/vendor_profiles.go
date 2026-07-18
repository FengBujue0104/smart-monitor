package smart

import "strings"

// ataVendorProfile captures only model/firmware semantics that have been
// verified against CrystalDiskInfo. Unknown models intentionally fall back to
// standard ATA names and raw values instead of guessing a vendor meaning.
type ataVendorProfile struct {
	name            string
	matches         func(string) bool
	aliases         map[int]string
	counterUnits    map[int]ATACounterUnit
	temperatureIDs  map[int]bool
	remainingHealth func([]Attr) (int, bool)
}

func ataVendorProfileForModel(model string) *ataVendorProfile {
	for i := range ataVendorProfiles {
		if ataVendorProfiles[i].matches(model) {
			return &ataVendorProfiles[i]
		}
	}
	return nil
}

func attrByID(attrs []Attr, id int) (Attr, bool) {
	for _, a := range attrs {
		if a.ID == id {
			return a, true
		}
	}
	return Attr{}, false
}

func kioxiaRemainingHealth(attrs []Attr) (int, bool) {
	a, ok := attrByID(attrs, 0xAD)
	if !ok {
		return 0, false
	}
	life := a.Value - 100
	return life, life > 0 && life <= 100
}

func wdBlueSA510RemainingHealth(attrs []Attr) (int, bool) {
	a, ok := attrByID(attrs, 0xE6)
	if !ok {
		return 0, false
	}
	life := 100 - int((a.Raw>>8)&0xFF)
	return life, life >= 0 && life <= 100
}

func samsungRemainingHealth(attrs []Attr) (int, bool) {
	a, ok := attrByID(attrs, 0xB1)
	if !ok {
		return 0, false
	}
	return a.Value, a.Value >= 0 && a.Value <= 100
}

func rawE9RemainingHealth(attrs []Attr) (int, bool) {
	a, ok := attrByID(attrs, 0xE9)
	if !ok {
		return 0, false
	}
	life := int(a.Raw & 0xFF)
	return life, life >= 0 && life <= 100
}

func incrementingRawE9RemainingHealth(attrs []Attr) (int, bool) {
	a, ok := attrByID(attrs, 0xE9)
	if !ok {
		return 0, false
	}
	life := 100 - int(a.Raw&0xFF)
	return life, life >= 0 && life <= 100
}

func currentE9RemainingHealth(attrs []Attr) (int, bool) {
	a, ok := attrByID(attrs, 0xE9)
	if !ok {
		return 0, false
	}
	return a.Value, a.Value >= 0 && a.Value <= 100
}

func sandiskRemainingHealth(attrs []Attr) (int, bool) {
	a, ok := attrByID(attrs, 0xE6)
	if !ok {
		return 0, false
	}
	life := 100 - int((a.Raw>>8)&0xFF)
	return life, life >= 0 && life <= 100
}

func currentCARemainingHealth(attrs []Attr) (int, bool) {
	a, ok := attrByID(attrs, 0xCA)
	if !ok {
		return 0, false
	}
	return a.Value, a.Value >= 0 && a.Value <= 100
}

func isSKHynixSATA(model string) bool {
	m := strings.ToUpper(model)
	return strings.Contains(m, "SK HYNIX") || strings.HasPrefix(m, "HFS") || strings.HasPrefix(m, "SHG")
}

func isSanDiskSATA(model string) bool {
	m := strings.ToLower(model)
	return strings.Contains(m, "sandisk") && !strings.Contains(m, "ssd p4") && !strings.Contains(m, "issd p4")
}

func isSanDisk512BCounterModel(model string) bool {
	m := strings.ToUpper(model)
	return strings.Contains(m, "X400") || strings.Contains(m, "X300") || strings.Contains(m, "X110") || strings.Contains(m, "SD5") || (strings.Contains(m, "X600") && strings.Contains(m, "2280"))
}

func isMicron512BCounterModel(model string) bool {
	m := strings.ToUpper(model)
	for _, family := range []string{"MICRON_M600", "MICRON M600", "MICRON_M550", "MICRON M550", "MICRON_M510", "MICRON M510", "MICRON_M500", "MICRON M500", "MICRON_1300", "MICRON 1300", "MICRON_1100", "MICRON 1100", "MTFDDA"} {
		if strings.Contains(m, family) {
			return true
		}
	}
	return false
}

func isMicronSATA(model string) bool {
	m := strings.ToUpper(model)
	return strings.Contains(m, "MICRON") || strings.Contains(m, "MTFD")
}

func isCrucialSATA(model string) bool {
	return strings.Contains(strings.ToUpper(model), "CRUCIAL")
}

var ataVendorProfiles = []ataVendorProfile{
	{
		name:           "YMTC/ZHITAI SATA",
		matches:        IsYMTCSATAModel,
		aliases:        map[int]string{0xF1: "Host_Writes", 0xF2: "Host_Reads", 0xF3: "Temperature_Celsius"},
		counterUnits:   map[int]ATACounterUnit{0xF1: ATACounterUnit512B, 0xF2: ATACounterUnit512B},
		temperatureIDs: map[int]bool{0xF3: true},
	},
	{
		name:    "KIOXIA SATA",
		matches: func(model string) bool { return strings.Contains(strings.ToLower(model), "kioxia") },
		aliases: map[int]string{
			0xA7: "SSD_Protection_Mode", 0xA8: "SATA_PHY_Error_Count", 0xA9: "Bad_Block_Count",
			0xAD: "Erase_Count_User_Data", 0xC0: "Unexpected_Power_Loss_Count", 0xF1: "Host_Writes",
		},
		counterUnits:    map[int]ATACounterUnit{0xF1: ATACounterUnit32MB},
		remainingHealth: kioxiaRemainingHealth,
	},
	{
		name:    "WD Blue SA510",
		matches: func(model string) bool { return strings.Contains(strings.ToLower(model), "wd blue sa510") },
		aliases: map[int]string{
			0xA5: "Block_Erase_Count_SLC", 0xA6: "Min_PE_Cycles", 0xA7: "Max_Bad_Blocks_Per_Die", 0xA8: "Max_PE_Cycles",
			0xA9: "Total_Bad_Blocks", 0xAA: "New_Bad_Blocks", 0xAB: "Program_Fail_Count", 0xAC: "Erase_Fail_Count",
			0xAD: "Average_PE_Cycles", 0xAE: "Unexpected_Power_Loss_Count", 0xE6: "Media_Wearout_Indicator",
			0xE8: "Available_Reserved_Space", 0xE9: "NAND_Writes", 0xEA: "NAND_Writes_SLC",
			0xF1: "Host_Writes_GB", 0xF2: "Host_Reads_GB", 0xF4: "Thermal_Throttle_Status",
		},
		counterUnits:    map[int]ATACounterUnit{0xE9: ATACounterUnitGB, 0xF1: ATACounterUnitGB, 0xF2: ATACounterUnitGB},
		remainingHealth: wdBlueSA510RemainingHealth,
	},
	{
		name:    "WD Blue SATA",
		matches: func(model string) bool { return strings.Contains(strings.ToLower(model), "wd blue") },
		aliases: map[int]string{
			0xA5: "Block_Erase_Count_SLC", 0xA6: "Min_PE_Cycles", 0xA7: "Max_Bad_Blocks_Per_Die", 0xA8: "Max_PE_Cycles",
			0xA9: "Total_Bad_Blocks", 0xAA: "New_Bad_Blocks", 0xAB: "Program_Fail_Count", 0xAC: "Erase_Fail_Count",
			0xAD: "Average_PE_Cycles", 0xAE: "Unexpected_Power_Loss_Count", 0xE6: "Media_Wearout_Indicator",
			0xE8: "Available_Reserved_Space", 0xE9: "NAND_Writes", 0xEA: "NAND_Writes_SLC",
			0xF1: "Host_Writes_GB", 0xF2: "Host_Reads_GB", 0xF4: "Thermal_Throttle_Status",
		},
	},
	{
		name:    "Samsung SATA SSD",
		matches: IsSamsungSATASSDModel,
		aliases: map[int]string{
			0xB1: "Wear_Leveling_Count", 0xB3: "Used_Reserved_Block_Count", 0xB5: "Program_Fail_Count",
			0xB6: "Erase_Fail_Count", 0xE8: "Available_Reserved_Space", 0xF1: "Host_Writes", 0xF2: "Host_Reads",
		},
		counterUnits:    map[int]ATACounterUnit{0xF1: ATACounterUnit512B, 0xF2: ATACounterUnit512B},
		remainingHealth: samsungRemainingHealth,
	},
	{
		name:            "Crucial MX/BX SATA",
		matches:         IsCrucial32MBHostCounterModel,
		aliases:         map[int]string{0xCA: "SSD_Life_Left", 0xF1: "Host_Writes", 0xF2: "Host_Reads"},
		counterUnits:    map[int]ATACounterUnit{0xF1: ATACounterUnit32MB, 0xF2: ATACounterUnit32MB},
		remainingHealth: currentCARemainingHealth,
	},
	{
		// CrystalDiskInfo identifies the remaining Crucial SATA families as
		// Micron-derived drives. CA is their normalized remaining-life field,
		// but the F1/F2 unit is not uniform across those older families.
		name:            "Crucial SATA",
		matches:         isCrucialSATA,
		aliases:         map[int]string{0xCA: "SSD_Life_Left"},
		remainingHealth: currentCARemainingHealth,
	},
	{
		name:            "Micron SATA 512 B",
		matches:         isMicron512BCounterModel,
		aliases:         map[int]string{0xCA: "SSD_Life_Left", 0xF1: "Host_Writes", 0xF2: "Host_Reads"},
		counterUnits:    map[int]ATACounterUnit{0xF1: ATACounterUnit512B, 0xF2: ATACounterUnit512B},
		remainingHealth: currentCARemainingHealth,
	},
	{
		name:            "Micron SATA",
		matches:         isMicronSATA,
		aliases:         map[int]string{0xCA: "SSD_Life_Left"},
		remainingHealth: currentCARemainingHealth,
	},
	{
		name:         "Intel/Solidigm SATA",
		matches:      IsIntelOrSolidigmSATAModel,
		aliases:      map[int]string{0xF1: "Host_Writes", 0xF3: "NAND_Writes"},
		counterUnits: map[int]ATACounterUnit{0xF1: ATACounterUnit32MB, 0xF3: ATACounterUnit32MB},
	},
	{
		name:         "Kingston KC600",
		matches:      IsKingstonKC600Model,
		aliases:      map[int]string{0xF1: "Host_Writes", 0xF2: "Host_Reads"},
		counterUnits: map[int]ATACounterUnit{0xF1: ATACounterUnit32MB, 0xF2: ATACounterUnit32MB},
	},
	{
		name:         "Toshiba SATA 32 MB",
		matches:      IsToshiba32MBHostCounterModel,
		aliases:      map[int]string{0xF1: "Host_Writes", 0xF2: "Host_Reads"},
		counterUnits: map[int]ATACounterUnit{0xF1: ATACounterUnit32MB, 0xF2: ATACounterUnit32MB},
	},
	{
		name: "SK hynix SC311/SC401",
		matches: func(model string) bool {
			m := strings.ToUpper(model)
			return strings.Contains(m, "SC311") || strings.Contains(m, "SC401")
		},
		aliases:         map[int]string{0xE9: "SSD_Life_Left", 0xF1: "Host_Writes", 0xF2: "Host_Reads"},
		counterUnits:    map[int]ATACounterUnit{0xF1: ATACounterUnit512B, 0xF2: ATACounterUnit512B},
		remainingHealth: rawE9RemainingHealth,
	},
	{
		name: "SK hynix HFS TND/MND",
		matches: func(model string) bool {
			m := strings.ToUpper(model)
			return strings.Contains(m, "HFS") && (strings.Contains(m, "TND") || strings.Contains(m, "MND"))
		},
		aliases:         map[int]string{0xE9: "SSD_Life_Left", 0xF1: "Host_Writes_GB", 0xF2: "Host_Reads_GB"},
		counterUnits:    map[int]ATACounterUnit{0xF1: ATACounterUnitGB, 0xF2: ATACounterUnitGB},
		remainingHealth: incrementingRawE9RemainingHealth,
	},
	{
		name: "SK hynix HFS TNF",
		matches: func(model string) bool {
			m := strings.ToUpper(model)
			return strings.Contains(m, "HFS") && strings.Contains(m, "TNF")
		},
		aliases:         map[int]string{0xE9: "SSD_Life_Left", 0xF1: "Host_Writes_GB", 0xF2: "Host_Reads_GB"},
		counterUnits:    map[int]ATACounterUnit{0xF1: ATACounterUnitGB, 0xF2: ATACounterUnitGB},
		remainingHealth: rawE9RemainingHealth,
	},
	{
		name:            "SK hynix SATA",
		matches:         isSKHynixSATA,
		aliases:         map[int]string{0xE9: "SSD_Life_Left", 0xF1: "Host_Writes_GB", 0xF2: "Host_Reads_GB"},
		counterUnits:    map[int]ATACounterUnit{0xF1: ATACounterUnitGB, 0xF2: ATACounterUnitGB},
		remainingHealth: currentE9RemainingHealth,
	},
	{
		name:    "SanDisk SATA 512 B",
		matches: func(model string) bool { return isSanDiskSATA(model) && isSanDisk512BCounterModel(model) },
		aliases: map[int]string{
			0xE6: "Media_Wearout_Indicator", 0xF1: "Host_Writes", 0xF2: "Host_Reads",
		},
		counterUnits:    map[int]ATACounterUnit{0xF1: ATACounterUnit512B, 0xF2: ATACounterUnit512B},
		remainingHealth: sandiskRemainingHealth,
	},
	{
		name:    "SanDisk SATA GB",
		matches: isSanDiskSATA,
		aliases: map[int]string{
			0xE6: "Media_Wearout_Indicator", 0xF1: "Host_Writes_GB", 0xF2: "Host_Reads_GB",
		},
		counterUnits:    map[int]ATACounterUnit{0xF1: ATACounterUnitGB, 0xF2: ATACounterUnitGB},
		remainingHealth: sandiskRemainingHealth,
	},
	{
		name: "Western Digital SATA",
		matches: func(model string) bool {
			m := strings.ToLower(model)
			return strings.Contains(m, "western digital") || strings.Contains(m, "wd ")
		},
		aliases: map[int]string{0xC4: "Reallocation_Event_Count", 0xF1: "Total_LBAs_Written", 0xF2: "Total_LBAs_Read"},
	},
	{
		name:    "Seagate",
		matches: func(model string) bool { return strings.Contains(strings.ToLower(model), "seagate") },
		aliases: map[int]string{
			0xBC: "Command_Timeout", 0xE9: "NAND_Writes", 0xEA: "NAND_Writes_SLC",
			0xF1: "Host_Writes_GB", 0xF2: "Host_Reads_GB",
		},
		counterUnits: map[int]ATACounterUnit{0xE9: ATACounterUnitGB, 0xEA: ATACounterUnitGB, 0xF1: ATACounterUnitGB, 0xF2: ATACounterUnitGB},
	},
}
