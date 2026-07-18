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
		name:         "Crucial MX/BX SATA",
		matches:      IsCrucial32MBHostCounterModel,
		aliases:      map[int]string{0xF1: "Host_Writes", 0xF2: "Host_Reads"},
		counterUnits: map[int]ATACounterUnit{0xF1: ATACounterUnit32MB, 0xF2: ATACounterUnit32MB},
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
