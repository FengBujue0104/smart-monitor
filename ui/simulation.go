package ui

import "smonitor/smart"

// SimulatedFailureDisks returns deterministic in-memory ATA and NVMe failure
// fixtures for validating the GUI. It never queries or writes to real disks.
func SimulatedFailureDisks() []smart.Disk {
	return []smart.Disk{
		{
			Index: 10,
			Kind:  smart.KindATA,
			// The model intentionally matches the supported WD Blue SA510 rule,
			// so this GUI fixture also verifies its remaining-life warning.
			Model: "SIMULATED WD Blue SA510 FAILURE",
			Attrs: []smart.Attr{
				{ID: 0x05, Name: "Reallocated_Sector_Ct", Raw: 2, Kind: "ata"},
				{ID: 0xC2, Name: "Temperature_Celsius", Raw: 61, Kind: "ata"},
				{ID: 0xBC, Name: "Command_Timeout", Raw: 11, Kind: "ata"},
				{ID: 0xE6, Name: "Media_Wearout_Indicator", Raw: 0x5F00, Kind: "ata"}, // 5% remaining
				{ID: 0xAA, Name: "Vendor_PreFail", Flags: 0x0001, Value: 10, Thresh: 10, Kind: "ata"},
				{ID: 0x09, Name: "Power_On_Hours", Raw: 1000, Kind: "ata"},
			},
		},
		{
			Index: 11,
			Kind:  smart.KindNVMe,
			Model: "SIMULATED NVME FAILURE",
			Attrs: []smart.Attr{
				{ID: smart.NVMeCriticalWarning, Name: "Critical_Warning", Raw: 0x02, Kind: "nvme"},
				{ID: smart.NVMeMediaErrors, Name: "Media_Data_Integrity_Errors", Raw: 1, Kind: "nvme"},
				{ID: smart.NVMeTemperatureSensor1, Name: "Temperature_Sensor_1_Kelvin", Raw: 334, Kind: "nvme"},
				{ID: smart.NVMePercentUsed, Name: "Percentage_Used", Raw: 80, Kind: "nvme"},
			},
		},
	}
}
