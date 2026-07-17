package smart

import (
	"encoding/binary"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// ===== NVMe 结构体（NVMe Base Spec 1.4/2.0）=====

// STORAGE_PROTOCOL_COMMAND 的固定部分为 56 字节，后面是 64 字节 NVMe 命令。
// 数据缓冲区通过 DataFromDeviceBufferOffset 指定，不能简单按「头 + 数据」拼接。

const (
	STORAGE_PROTOCOL_TYPE_NVMe = 0x01
	NVMeGetLogPage             = 0x02
	NVMeLogID_SMART_Health     = 0x02
)

// buildNVMeGetLogPage 构造 Get Log Page 命令缓冲区。
// 返回 [命令头][数据缓冲区] 拼接后的字节。
func buildNVMeGetLogPage(logID uint8, lenBytes int) []byte {
	const (
		protocolHeaderSize = 56
		commandOffset      = 56
		dataOffset         = 128
		commandLength      = 64
	)
	if lenBytes <= 0 || lenBytes%4 != 0 {
		return nil
	}
	buf := make([]byte, dataOffset+lenBytes)
	// STORAGE_PROTOCOL_COMMAND
	binary.LittleEndian.PutUint32(buf[0x00:0x04], 1) // Version
	binary.LittleEndian.PutUint32(buf[0x04:0x08], protocolHeaderSize)
	binary.LittleEndian.PutUint32(buf[0x08:0x0C], STORAGE_PROTOCOL_TYPE_NVMe)
	binary.LittleEndian.PutUint32(buf[0x18:0x1C], commandLength)
	binary.LittleEndian.PutUint32(buf[0x20:0x24], uint32(lenBytes)) // DataFromDeviceTransferLength
	binary.LittleEndian.PutUint32(buf[0x24:0x28], dataOffset)       // DataFromDeviceBufferOffset
	binary.LittleEndian.PutUint32(buf[0x30:0x34], 10)               // TimeoutValue

	// NVMe Get Log Page command. CDW10 contains NUMDL and LID.
	buf[commandOffset] = NVMeGetLogPage
	cdw10 := (uint32(lenBytes/4-1) << 16) | uint32(logID)
	binary.LittleEndian.PutUint32(buf[commandOffset+40:commandOffset+44], cdw10)
	return buf
}

// issueNVMeGetLogPage 在已打开的 NVMe 设备上发 Get Log Page 并返回数据缓冲区。
func issueNVMeGetLogPage(h windows.Handle, logID uint8) ([]byte, error) {
	buf := buildNVMeGetLogPage(logID, 512)
	if buf == nil {
		return nil, fmt.Errorf("invalid NVMe log length")
	}
	outBuf := make([]byte, len(buf))
	var bytesReturned uint32

	err := windows.DeviceIoControl(
		h,
		IOCTL_STORAGE_PROTOCOL_COMMAND,
		&buf[0],
		uint32(len(buf)),
		&outBuf[0],
		uint32(len(outBuf)),
		&bytesReturned,
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("IOCTL_STORAGE_PROTOCOL_COMMAND logID=0x%02X err=%w", logID, err)
	}
	const dataOffset = 128
	if bytesReturned < uint32(dataOffset+512) {
		return nil, fmt.Errorf("NVMe 返回过短: got=%d", bytesReturned)
	}
	return append([]byte(nil), outBuf[dataOffset:dataOffset+512]...), nil
}

// parseNVMeHealthLog 解析 NVMe SMART/Health Information Log Page（512 字节）。
// 布局（NVMe Base Spec Figure 207）：
//
//	Offset  Size  Field
//	0x00    1     CriticalWarning（位 0=可用备用不足,1=温度越限,2=可靠性降级,3=只读,4=易失备份失败）
//	0x01    2     Temperature（开尔文，uint16）
//	0x03    1     AvailableSpare（%）
//	0x04    1     AvailableSpareThreshold（%）
//	0x05    1     PercentageUsed（%）
//	0x06    1     EnduranceGroupCriticalWarningSummary
//	0x08    8     DataUnitsRead（1000 * 512B 单位）
//	0x10    8     DataUnitsWritten
//	0x18    8     HostReadCommands
//	0x20    8     HostWriteCommands
//	0x28    8     ControllerBusyTime（分钟）
//	0x30    8     PowerCycles
//	0x38    8     PowerOnHours
//	0x40    8     UnsafeShutdowns
//	0x48    8     MediaErrors（= 用户说的 0E "介质与数据完整性错误计数"）
//	0x50    8     NumErrorInfoLogEntries
//	0x58    4     WarningCompTempTime（分钟）
//	0x5C    4     CriticalCompTempTime（分钟）
//	0x60..  8*8   SensorTemperature[8]（开尔文）
func parseNVMeHealthLog(data []byte) []Attr {
	if len(data) < 0x58 {
		return nil
	}
	var attrs []Attr

	cw := data[0x00]
	tempK := binary.LittleEndian.Uint16(data[0x01:0x03])
	spare := data[0x03]
	spareThresh := data[0x04]
	pctUsed := data[0x05]
	mediaErrors := binary.LittleEndian.Uint64(data[0x48:0x50])
	dataUnitsRead := binary.LittleEndian.Uint64(data[0x08:0x10])
	dataUnitsWritten := binary.LittleEndian.Uint64(data[0x10:0x18])
	powerCycles := binary.LittleEndian.Uint64(data[0x30:0x38])
	powerOnHours := binary.LittleEndian.Uint64(data[0x38:0x40])
	unsafeShutdowns := binary.LittleEndian.Uint64(data[0x40:0x48])
	errorInfoEntries := binary.LittleEndian.Uint64(data[0x50:0x58])
	warningTempTime := uint64(binary.LittleEndian.Uint32(data[0x58:0x5C]))
	criticalTempTime := uint64(binary.LittleEndian.Uint32(data[0x5C:0x60]))

	attrs = append(attrs, Attr{ID: NVMeCriticalWarning, Name: "Critical_Warning", Raw: uint64(cw), Kind: "nvme"})
	attrs = append(attrs, Attr{ID: NVMeTemperature, Name: "Temperature_Kelvin", Raw: uint64(tempK), Kind: "nvme"})
	attrs = append(attrs, Attr{ID: NVMeAvailableSpare, Name: "Available_Spare_Pct", Raw: uint64(spare), Kind: "nvme"})
	attrs = append(attrs, Attr{ID: NVMeAvailSpareThresh, Name: "Available_Spare_Threshold", Raw: uint64(spareThresh), Kind: "nvme"})
	attrs = append(attrs, Attr{ID: NVMePercentUsed, Name: "Percentage_Used", Raw: uint64(pctUsed), Kind: "nvme"})
	attrs = append(attrs, Attr{ID: NVMeMediaErrors, Name: "Media_Data_Integrity_Errors", Raw: mediaErrors, Kind: "nvme"})
	attrs = append(attrs, Attr{ID: NVMeDataUnitsRead, Name: "Data_Units_Read", Raw: dataUnitsRead, Kind: "nvme"})
	attrs = append(attrs, Attr{ID: NVMeDataUnitsWritten, Name: "Data_Units_Written", Raw: dataUnitsWritten, Kind: "nvme"})
	attrs = append(attrs, Attr{ID: NVMePowerCycles, Name: "Power_Cycles", Raw: powerCycles, Kind: "nvme"})
	attrs = append(attrs, Attr{ID: NVMePowerOnHours, Name: "Power_On_Hours", Raw: powerOnHours, Kind: "nvme"})
	attrs = append(attrs, Attr{ID: NVMeUnsafeShutdowns, Name: "Unsafe_Shutdowns", Raw: unsafeShutdowns, Kind: "nvme"})
	attrs = append(attrs, Attr{ID: NVMeErrorInfoEntries, Name: "Error_Info_Log_Entries", Raw: errorInfoEntries, Kind: "nvme"})
	attrs = append(attrs, Attr{ID: NVMeWarningTempTime, Name: "Warning_Temperature_Time", Raw: warningTempTime, Kind: "nvme"})
	attrs = append(attrs, Attr{ID: NVMeCriticalTempTime, Name: "Critical_Temperature_Time", Raw: criticalTempTime, Kind: "nvme"})

	// 只读模式（CriticalWarning bit 3）
	if cw&(1<<3) != 0 {
		attrs = append(attrs, Attr{ID: NVMeReadOnly, Name: "Read_Only_Mode", Raw: 1, Kind: "nvme"})
	}
	return attrs
}

// ReadNVMeHealth 读取 NVMe 健康日志并返回统一 Attr 列表。
func ReadNVMeHealth(h windows.Handle) ([]Attr, error) {
	data, err := issueNVMeGetLogPage(h, NVMeLogID_SMART_Health)
	if err != nil {
		return nil, err
	}
	return parseNVMeHealthLog(data), nil
}

// NVMeIdentify 通过 IOCTL_STORAGE_QUERY_PROPERTY 获取 NVMe 型号/序列号（更可靠）。
// 这里返回由 enum.go 调用方填充，本文件只负责健康日志。
var _ = unsafe.Pointer(nil) // 保留 unsafe 导入
