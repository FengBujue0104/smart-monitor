package smart

import "strings"

// ATAAttrName 返回 ATA 属性 ID 的规范名（来源：smartmontools attributedictionary.h / ATA8-ACS / CrystalDiskInfo）。
// 注：0x0E 与 0xBC 在 ATA 规范中未统一定义（厂商特定），保留最常见厂牌含义。
func ATAAttrName(id int) string {
	if n, ok := dict[id]; ok {
		return n
	}
	return "Unknown_0x" + uitoa(id)
}

// ATAAttrNameForModel supplies common vendor-specific aliases without changing
// the numeric SMART attribute identity.
func ATAAttrNameForModel(model string, id int) string {
	name := ATAAttrName(id)
	if profile := ataVendorProfileForModel(model); profile != nil {
		if alias, ok := profile.aliases[id]; ok {
			return alias
		}
	}
	m := strings.ToLower(model)
	switch {
	case isYMTCSATA(model):
		// CrystalDiskInfo AtaSmart.cpp: ZHITAI (YMTC) models use F3 for
		// the current temperature, unlike other vendors that reuse this ID.
		switch id {
		case 0xF1:
			return "Host_Writes"
		case 0xF2:
			return "Host_Reads"
		case 0xF3:
			return "Temperature_Celsius"
		}
	case strings.Contains(m, "kioxia"):
		switch id {
		case 0xA7:
			return "SSD_Protection_Mode"
		case 0xA8:
			return "SATA_PHY_Error_Count"
		case 0xA9:
			return "Bad_Block_Count"
		case 0xAD:
			return "Erase_Count_User_Data"
		case 0xC0:
			return "Unexpected_Power_Loss_Count"
		case 0xF1:
			return "Host_Writes"
		}
	case strings.Contains(m, "wd blue"):
		switch id {
		case 0xA5:
			return "Block_Erase_Count_SLC"
		case 0xA6:
			return "Min_PE_Cycles"
		case 0xA7:
			return "Max_Bad_Blocks_Per_Die"
		case 0xA8:
			return "Max_PE_Cycles"
		case 0xA9:
			return "Total_Bad_Blocks"
		case 0xAA:
			return "New_Bad_Blocks"
		case 0xAB:
			return "Program_Fail_Count"
		case 0xAC:
			return "Erase_Fail_Count"
		case 0xAD:
			return "Average_PE_Cycles"
		case 0xAE:
			return "Unexpected_Power_Loss_Count"
		case 0xE6:
			return "Media_Wearout_Indicator"
		case 0xE8:
			return "Available_Reserved_Space"
		case 0xE9:
			return "NAND_Writes"
		case 0xEA:
			return "NAND_Writes_SLC"
		case 0xF1:
			return "Host_Writes_GB"
		case 0xF2:
			return "Host_Reads_GB"
		case 0xF4:
			return "Thermal_Throttle_Status"
		}
	case IsSamsungSATASSDModel(model):
		switch id {
		case 0xB1:
			return "Wear_Leveling_Count"
		case 0xB3:
			return "Used_Reserved_Block_Count"
		case 0xB5:
			return "Program_Fail_Count"
		case 0xB6:
			return "Erase_Fail_Count"
		case 0xE8:
			return "Available_Reserved_Space"
		case 0xF1:
			return "Host_Writes"
		case 0xF2:
			return "Host_Reads"
		}
	case IsCrucial32MBHostCounterModel(model):
		switch id {
		case 0xF1:
			return "Host_Writes"
		case 0xF2:
			return "Host_Reads"
		}
	case IsIntelOrSolidigmSATAModel(model):
		switch id {
		case 0xF1:
			return "Host_Writes"
		case 0xF3:
			return "NAND_Writes"
		}
	case IsKingstonKC600Model(model):
		switch id {
		case 0xF1:
			return "Host_Writes"
		case 0xF2:
			return "Host_Reads"
		}
	case IsToshiba32MBHostCounterModel(model):
		switch id {
		case 0xF1:
			return "Host_Writes"
		case 0xF2:
			return "Host_Reads"
		}
	case strings.Contains(m, "western digital") || strings.Contains(m, "wd "):
		switch id {
		case 0xC4:
			return "Reallocation_Event_Count"
		case 0xF1:
			return "Total_LBAs_Written"
		case 0xF2:
			return "Total_LBAs_Read"
		}
	case strings.Contains(m, "seagate"):
		switch id {
		case 0xBC:
			return "Command_Timeout"
		case 0xE9:
			return "NAND_Writes"
		case 0xEA:
			return "NAND_Writes_SLC"
		case 0xF1:
			return "Host_Writes_GB"
		case 0xF2:
			return "Host_Reads_GB"
		}
	}
	return name
}

// ATATemperatureAttributeForModel identifies attributes whose low raw byte is
// the current Celsius temperature. F3 is deliberately model-scoped: according
// to CrystalDiskInfo, it is a temperature only on ZHITAI/YMTC SATA SSDs.
func ATATemperatureAttributeForModel(model string, id int) bool {
	if profile := ataVendorProfileForModel(model); profile != nil && profile.temperatureIDs[id] {
		return true
	}
	switch id {
	case 0xB9, 0xBE, 0xC2:
		return true
	case 0xF3:
		return isYMTCSATA(model)
	default:
		return false
	}
}

func isYMTCSATA(model string) bool {
	return IsYMTCSATAModel(model)
}

// IsYMTCSATAModel reports whether the model matches CrystalDiskInfo's YMTC
// (ZHITAI/致态) SATA SSD identification rule.
func IsYMTCSATAModel(model string) bool {
	m := strings.ToLower(model)
	return strings.Contains(m, "zhitai") || strings.Contains(model, "致态")
}

// IsSamsungSATASSDModel limits Samsung SMART interpretations to models that
// identify as SSDs. This avoids applying SSD-only B1/F1/F2 meanings to older
// Samsung hard drives, whose vendor attributes have different semantics.
func IsSamsungSATASSDModel(model string) bool {
	m := strings.ToLower(strings.TrimSpace(model))
	return strings.Contains(m, "samsung ssd") || strings.Contains(m, "samsung mz") || strings.HasPrefix(m, "mz-")
}

// IsCrucial32MBHostCounterModel matches the Crucial SATA families for which
// CrystalDiskInfo's IsSsdMicronMU03 selects 32 MB host read/write units.
func IsCrucial32MBHostCounterModel(model string) bool {
	m := strings.ToUpper(model)
	for _, family := range []string{"MX500SSD", "BX500SSD", "MX300SSD", "BX300SSD", "MX200SSD", "BX200SSD", "MX100SSD", "BX100SSD"} {
		if strings.Contains(m, family) {
			return true
		}
	}
	return false
}

// IsIntelOrSolidigmSATAModel matches the vendor identification used by
// CrystalDiskInfo's Intel SMART handling. NVMe devices use a separate path.
func IsIntelOrSolidigmSATAModel(model string) bool {
	m := strings.ToLower(model)
	return strings.Contains(m, "intel") || strings.Contains(m, "solidigm")
}

// IsKingstonKC600Model matches CrystalDiskInfo's Kingston KC600 branch,
// which explicitly declares 32 MB host read/write units.
func IsKingstonKC600Model(model string) bool {
	return strings.Contains(strings.ToUpper(model), "KC600")
}

// IsToshiba32MBHostCounterModel matches the explicit Toshiba SATA families
// for which CrystalDiskInfo's IsSsdToshiba selects 32 MB host I/O units.
func IsToshiba32MBHostCounterModel(model string) bool {
	m := strings.ToUpper(model)
	for _, family := range []string{"THNSNC", "THNSNJ", "THNSNK", "KSG60", "TL100", "TR150", "TR200"} {
		if strings.Contains(m, family) {
			return true
		}
	}
	return false
}

// ATACounterUnit is the unit CrystalDiskInfo assigns to a vendor-specific ATA
// counter after matching the model and attribute ID.
type ATACounterUnit uint8

const (
	ATACounterUnitUnknown ATACounterUnit = iota
	ATACounterUnitGB
	ATACounterUnit32MB
	ATACounterUnit512B
)

// ATACounterUnitForModel centralizes source-verified CrystalDiskInfo counter
// units. It intentionally returns false for unknown model/attribute pairs.
func ATACounterUnitForModel(model string, id int) (ATACounterUnit, bool) {
	if profile := ataVendorProfileForModel(model); profile != nil {
		unit, ok := profile.counterUnits[id]
		return unit, ok
	}
	return ATACounterUnitUnknown, false
}

// ATAHealthPercentForModel returns a vendor-defined remaining-life percentage
// only where the interpretation is verified by CrystalDiskInfo and a model
// match. The ATA standard does not assign a universal SSD-life attribute.
func ATAHealthPercentForModel(model string, attrs []Attr) (int, bool) {
	if profile := ataVendorProfileForModel(model); profile != nil && profile.remainingHealth != nil {
		return profile.remainingHealth(attrs)
	}
	return 0, false
}

func uitoa(v int) string {
	const hex = "0123456789ABCDEF"
	if v < 16 {
		return string([]byte{hex[v]})
	}
	var b [8]byte
	n := 8
	for v > 0 {
		n--
		b[n] = hex[v%16]
		v /= 16
	}
	return string(b[n:])
}

// dict: 常用 ATA 属性 ID -> 规范名（完整表覆盖 0x01..0xFF 中已知项）。
var dict = map[int]string{
	0x01: "Raw_Read_Error_Rate",
	0x02: "Throughput_Performance",
	0x03: "Spin_Up_Time",
	0x04: "Start_Stop_Count",
	0x05: "Reallocated_Sector_Ct",
	0x06: "Read_Channel_Margin",
	0x07: "Seek_Error_Rate",
	0x08: "Seek_Time_Performance",
	0x09: "Power_On_Hours",
	0x0A: "Spin_Retry_Count",
	0x0B: "Calibration_Retry_Count",
	0x0C: "Power_Cycle_Count",
	0x0E: "Device_Was_Thermal_Count",
	0x10: "Soft_ECC_Correction",
	0x22: "Helium_Level",
	0xA7: "SSD_Reserve_Backup_Block",
	0xA9: "Unknown_SSD",
	0xAA: "Used_Rsvd_Blk_Cnt_Chip",
	0xAB: "Program_Fail_Cnt_Chip",
	0xAC: "Head_Amplitude",
	0xAD: "Wear_Leveling_Count",
	0xAE: "Unknown",
	0xAF: "Unknown",
	0xB0: "Unused_Reserved_Block_Ct",
	0xB1: "Wear_Leveling_Count2",
	0xB2: "Deleted_Sector_Ct",
	0xB3: "Soft_Read_Error_Rate",
	0xB4: "Unused_Reserved_Block_Ct2",
	0xB5: "Program_Fail_Count",
	0xB6: "Multi-Zone_Error_Rate",
	0xB7: "SATA_Downshift_Error_Count",
	0xB8: "End-to-End_Error",
	0xB9: "Temperature_Celsius_B",
	0xBA: "High_Fly_Writes",
	0xBB: "Reported_Uncorrectable_Errors",
	0xBC: "Command_Timeout",
	0xBD: "G-Sense_Error_Rate",
	0xBE: "Airflow_Temperature_Cel",
	0xBF: "G-Sense_Error_Rate2",
	0xC0: "Power-off_Retract_Count",
	0xC1: "Load_Cycle_Count",
	0xC2: "Temperature_Celsius",
	0xC3: "Hardware_ECC_Recovered",
	0xC4: "Reallocation_Event_Count",
	0xC5: "Current_Pending_Sector",
	0xC6: "Offline_Uncorrectable",
	0xC7: "UDMA_CRC_Error_Count",
	0xC8: "Write_Error_Rate",
	0xC9: "Soft_Read_Error_Rate2",
	0xCA: "Data_Address_Mark_Errs",
	0xCB: "Run_Out_Cancel",
	0xCC: "Soft_ECC_Correction2",
	0xCD: "Thermal_Asperity_Rate",
	0xCE: "Flying_Height",
	0xCF: "Spin_High_Current",
	0xD0: "Spin_Buzz",
	0xD1: "Offline_Seek_Performance",
	0xD3: "Vibration_During_Write",
	0xD4: "Shock_During_Write",
	0xDC: "Disk_Shift",
	0xDD: "G-Sense_Error_Rate3",
	0xDE: "Loaded_Hours",
	0xDF: "Load_Unload_Retry_Count",
	0xE0: "Load_Friction",
	0xE1: "Load_Unload_Cycle_Count",
	0xE2: "Load_In-time",
	0xE3: "Torq-amp_Count",
	0xE4: "Power-Off_Retract_Cycle",
	0xE6: "GMR_Head_Amplitude",
	0xE7: "SSD_Life_Left",
	0xE8: "Available_Reservd_Space",
	0xE9: "Media_Wearout_Indicator",
	0xEA: "Average_Erase_Count",
	0xEB: "Good_Block_Count",
	0xF1: "Total_LBAs_Written",
	0xF2: "Total_LBAs_Read",
}
