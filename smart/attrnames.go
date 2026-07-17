package smart

// ATAAttrName 返回 ATA 属性 ID 的规范名（来源：smartmontools attributedictionary.h / ATA8-ACS / CrystalDiskInfo）。
// 注：0x0E 与 0xBC 在 ATA 规范中未统一定义（厂商特定），保留最常见厂牌含义。
func ATAAttrName(id int) string {
	if n, ok := dict[id]; ok {
		return n
	}
	return "Unknown_0x" + uitoa(id)
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
