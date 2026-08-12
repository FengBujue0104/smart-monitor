package smart

import (
	"encoding/binary"
	"fmt"

	"golang.org/x/sys/windows"
)

// ===== NVMe 结构体（NVMe Base Spec 1.4/2.0）=====

// STORAGE_PROTOCOL_COMMAND 的固定部分为 80 字节，后面是 64 字节 NVMe 命令。
// 数据缓冲区通过 DataFromDeviceBufferOffset 指定，不能简单按「头 + 数据」拼接。

const (
	// STORAGE_PROTOCOL_TYPE（SDK ntddstor.h）：ProtocolTypeScsi=1, ProtocolTypeAta=2,
	// ProtocolTypeNvme=3。曾误写为 0x01（Scsi），会把 NVMe 命令错误地标识为 SCSI，
	// 部分控制器/驱动会拒绝或走回退路径。
	STORAGE_PROTOCOL_TYPE_NVMe = 0x03
	storageProtocolTypeNVMe     = 0x03 // STORAGE_PROTOCOL_TYPE.ProtocolTypeNvme（property 查询路径）
	storageDeviceProtocolQuery  = 0x32 // StorageDeviceProtocolSpecificProperty
	nvmeDataTypeLogPage         = 0x02
	NVMeGetLogPage              = 0x02
	NVMeIdentify                = 0x06
	NVMeLogID_SMART_Health      = 0x02
	NVMeIdentifyController      = 0x01
	nvmeIdentifyControllerBytes = 4096
)

const (
	storagePropertyQueryHeaderBytes = 8
	storageProtocolSpecificDataSize = 40
	nvmeHealthPropertyResponseBytes = storagePropertyQueryHeaderBytes + storageProtocolSpecificDataSize
)

// buildNVMeGetLogPage 构造 Get Log Page 命令缓冲区。
// 返回 [命令头][数据缓冲区] 拼接后的字节。
func buildNVMeGetLogPage(logID uint8, lenBytes int) []byte {
	if lenBytes <= 0 || lenBytes%4 != 0 {
		return nil
	}
	cdw10 := (uint32(lenBytes/4-1) << 16) | uint32(logID)
	return buildNVMeAdminCommand(NVMeGetLogPage, lenBytes, cdw10)
}

// buildNVMeAdminCommand builds a STORAGE_PROTOCOL_COMMAND containing an NVMe
// admin command with a data-in buffer. Both Get Log Page and Identify use this
// standard Windows transport.
func buildNVMeAdminCommand(opcode uint8, lenBytes int, cdw10 uint32) []byte {
	const (
		protocolHeaderSize = 80
		commandOffset      = 80
		dataOffset         = 144
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
	binary.LittleEndian.PutUint32(buf[0x24:0x28], uint32(lenBytes)) // DataFromDeviceTransferLength
	binary.LittleEndian.PutUint32(buf[0x34:0x38], dataOffset)       // DataFromDeviceBufferOffset
	binary.LittleEndian.PutUint32(buf[0x28:0x2C], 10)               // TimeoutValue
	binary.LittleEndian.PutUint32(buf[0x38:0x3C], STORAGE_PROTOCOL_SPECIFIC_NVME_ADMIN_COMMAND)

	buf[commandOffset] = opcode
	binary.LittleEndian.PutUint32(buf[commandOffset+40:commandOffset+44], cdw10)
	return buf
}

// issueNVMeGetLogPage 在已打开的 NVMe 设备上发 Get Log Page 并返回数据缓冲区。
func issueNVMeGetLogPage(h windows.Handle, logID uint8) ([]byte, error) {
	buf := buildNVMeGetLogPage(logID, 512)
	return issueNVMeAdminCommand(h, buf, 512, fmt.Sprintf("logID=0x%02X", logID))
}

func issueNVMeIdentifyController(h windows.Handle) ([]byte, error) {
	buf := buildNVMeAdminCommand(NVMeIdentify, nvmeIdentifyControllerBytes, NVMeIdentifyController)
	return issueNVMeAdminCommand(h, buf, nvmeIdentifyControllerBytes, "identify-controller")
}

// buildNVMeHealthLogPropertyQuery builds STORAGE_PROPERTY_QUERY followed by
// STORAGE_PROTOCOL_SPECIFIC_DATA. This standard Windows transport is a
// read-only alternative to IOCTL_STORAGE_PROTOCOL_COMMAND.
func buildNVMeHealthLogPropertyQuery() []byte {
	buf := make([]byte, nvmeHealthPropertyResponseBytes)
	binary.LittleEndian.PutUint32(buf[0:4], storageDeviceProtocolQuery)
	binary.LittleEndian.PutUint32(buf[4:8], PropertyStandardQuery)
	protocol := buf[storagePropertyQueryHeaderBytes:]
	binary.LittleEndian.PutUint32(protocol[0:4], storageProtocolTypeNVMe)
	binary.LittleEndian.PutUint32(protocol[4:8], nvmeDataTypeLogPage)
	binary.LittleEndian.PutUint32(protocol[8:12], NVMeLogID_SMART_Health)
	binary.LittleEndian.PutUint32(protocol[16:20], storageProtocolSpecificDataSize)
	binary.LittleEndian.PutUint32(protocol[20:24], 512)
	return buf
}

func parseNVMeHealthLogPropertyResponse(buf []byte, returned uint32) ([]byte, error) {
	if returned < nvmeHealthPropertyResponseBytes || len(buf) < nvmeHealthPropertyResponseBytes {
		return nil, fmt.Errorf("NVMe property response too short: got=%d", returned)
	}
	protocol := buf[storagePropertyQueryHeaderBytes:]
	if binary.LittleEndian.Uint32(protocol[0:4]) != storageProtocolTypeNVMe ||
		binary.LittleEndian.Uint32(protocol[4:8]) != nvmeDataTypeLogPage {
		return nil, fmt.Errorf("unexpected NVMe property response type")
	}
	dataOffset := binary.LittleEndian.Uint32(protocol[16:20])
	dataLength := binary.LittleEndian.Uint32(protocol[20:24])
	if dataOffset < storageProtocolSpecificDataSize || dataLength < 512 {
		return nil, fmt.Errorf("invalid NVMe property data offset=%d length=%d", dataOffset, dataLength)
	}
	start := uint64(storagePropertyQueryHeaderBytes) + uint64(dataOffset)
	end := start + 512
	if end > uint64(returned) || end > uint64(len(buf)) {
		return nil, fmt.Errorf("NVMe property data outside response: offset=%d returned=%d", dataOffset, returned)
	}
	return append([]byte(nil), buf[start:end]...), nil
}

func issueNVMeHealthLogPropertyQuery(h windows.Handle) ([]byte, error) {
	query := buildNVMeHealthLogPropertyQuery()
	out := make([]byte, nvmeHealthPropertyResponseBytes+512)
	var returned uint32
	if err := windows.DeviceIoControl(h, IOCTL_STORAGE_QUERY_PROPERTY,
		&query[0], uint32(len(query)), &out[0], uint32(len(out)), &returned, nil); err != nil {
		return nil, fmt.Errorf("IOCTL_STORAGE_QUERY_PROPERTY NVMe Health Log: %w", err)
	}
	return parseNVMeHealthLogPropertyResponse(out, returned)
}

func issueNVMeAdminCommand(h windows.Handle, buf []byte, dataLen int, operation string) ([]byte, error) {
	if buf == nil {
		return nil, fmt.Errorf("invalid NVMe %s command", operation)
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
		return nil, fmt.Errorf("IOCTL_STORAGE_PROTOCOL_COMMAND %s err=%w", operation, err)
	}
	if err := parseNVMeProtocolStatus(outBuf); err != nil {
		return nil, fmt.Errorf("NVMe %s protocol status: %w", operation, err)
	}
	const dataOffset = 144
	if bytesReturned < uint32(dataOffset+dataLen) {
		return nil, fmt.Errorf("NVMe 返回过短: got=%d", bytesReturned)
	}
	return append([]byte(nil), outBuf[dataOffset:dataOffset+dataLen]...), nil
}

func parseNVMeProtocolStatus(buf []byte) error {
	if len(buf) < 0x18 {
		return fmt.Errorf("protocol response too short")
	}
	status := binary.LittleEndian.Uint32(buf[0x10:0x14])
	if status != 1 { // STORAGE_PROTOCOL_STATUS_SUCCESS
		code := binary.LittleEndian.Uint32(buf[0x14:0x18])
		return fmt.Errorf("return status=0x%08X error code=0x%08X", status, code)
	}
	return nil
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
//	0x20    16    DataUnitsRead（1000 * 512B 单位）
//	0x30    16    DataUnitsWritten
//	0x40    16    HostReadCommands
//	0x50    16    HostWriteCommands
//	0x60    16    ControllerBusyTime（分钟）
//	0x70    16    PowerCycles
//	0x80    16    PowerOnHours
//	0x90    16    UnsafeShutdowns
//	0xA0    16    MediaErrors（介质与数据完整性错误计数）
//	0xB0    16    NumErrorInfoLogEntries
//	0xC0    4     WarningCompTempTime（分钟）
//	0xC4    4     CriticalCompTempTime（分钟）
//	0xC8    8*2   SensorTemperature[8]（开尔文）
//
// 规范页为 512 字节，但部分控制器/桥接会返回截断页。核心健康字段
// （CriticalWarning..PercentageUsed 等）集中在开头，因此只要求前 0x07 字节；
// 缺失的可选计数器/温度时间按边界跳过，而不是把整页丢弃——否则一块日志
// 截断的盘会显示“SMART 未读取”却没有任何错误说明。
func parseNVMeHealthLog(data []byte) []Attr {
	if len(data) < 0x07 {
		return nil
	}
	var attrs []Attr

	cw := data[0x00]
	tempK := binary.LittleEndian.Uint16(data[0x01:0x03])
	spare := data[0x03]
	spareThresh := data[0x04]
	pctUsed := data[0x05]
	enduranceGroupCW := data[0x06]
	dataUnitsRead, dataUnitsReadHigh := nvmeUint128At(data, 0x20)
	dataUnitsWritten, dataUnitsWrittenHigh := nvmeUint128At(data, 0x30)
	powerCycles, powerCyclesHigh := nvmeUint128At(data, 0x70)
	powerOnHours, powerOnHoursHigh := nvmeUint128At(data, 0x80)
	unsafeShutdowns, unsafeShutdownsHigh := nvmeUint128At(data, 0x90)
	mediaErrors, mediaErrorsHigh := nvmeUint128At(data, 0xA0)
	errorInfoEntries, errorInfoEntriesHigh := nvmeUint128At(data, 0xB0)
	warningTempTime := nvmeUint32At(data, 0xC0)
	criticalTempTime := nvmeUint32At(data, 0xC4)

	attrs = append(attrs, Attr{ID: NVMeCriticalWarning, Name: "Critical_Warning", Raw: uint64(cw), Kind: "nvme"})
	attrs = append(attrs, Attr{ID: NVMeTemperature, Name: "Temperature_Kelvin", Raw: uint64(tempK), Kind: "nvme"})
	attrs = append(attrs, Attr{ID: NVMeAvailableSpare, Name: "Available_Spare_Pct", Raw: uint64(spare), Kind: "nvme"})
	attrs = append(attrs, Attr{ID: NVMeAvailSpareThresh, Name: "Available_Spare_Threshold", Raw: uint64(spareThresh), Kind: "nvme"})
	attrs = append(attrs, Attr{ID: NVMePercentUsed, Name: "Percentage_Used", Raw: uint64(pctUsed), Kind: "nvme"})
	attrs = append(attrs, Attr{ID: NVMeEnduranceGroupCriticalWarning, Name: "Endurance_Group_Critical_Warning_Summary", Raw: uint64(enduranceGroupCW), Kind: "nvme"})
	attrs = append(attrs, Attr{ID: NVMeMediaErrors, Name: "Media_Data_Integrity_Errors", Raw: mediaErrors, RawHigh: mediaErrorsHigh, Kind: "nvme"})
	attrs = append(attrs, Attr{ID: NVMeDataUnitsRead, Name: "Data_Units_Read", Raw: dataUnitsRead, RawHigh: dataUnitsReadHigh, Kind: "nvme"})
	attrs = append(attrs, Attr{ID: NVMeDataUnitsWritten, Name: "Data_Units_Written", Raw: dataUnitsWritten, RawHigh: dataUnitsWrittenHigh, Kind: "nvme"})
	attrs = append(attrs, Attr{ID: NVMePowerCycles, Name: "Power_Cycles", Raw: powerCycles, RawHigh: powerCyclesHigh, Kind: "nvme"})
	attrs = append(attrs, Attr{ID: NVMePowerOnHours, Name: "Power_On_Hours", Raw: powerOnHours, RawHigh: powerOnHoursHigh, Kind: "nvme"})
	attrs = append(attrs, Attr{ID: NVMeUnsafeShutdowns, Name: "Unsafe_Shutdowns", Raw: unsafeShutdowns, RawHigh: unsafeShutdownsHigh, Kind: "nvme"})
	attrs = append(attrs, Attr{ID: NVMeErrorInfoEntries, Name: "Error_Info_Log_Entries", Raw: errorInfoEntries, RawHigh: errorInfoEntriesHigh, Kind: "nvme"})
	attrs = append(attrs, Attr{ID: NVMeWarningTempTime, Name: "Warning_Temperature_Time", Raw: warningTempTime, Kind: "nvme"})
	attrs = append(attrs, Attr{ID: NVMeCriticalTempTime, Name: "Critical_Temperature_Time", Raw: criticalTempTime, Kind: "nvme"})
	// Temperature Sensor 1..8 are optional vendor-defined locations. Keep
	// health reporting aligned with CrystalDiskInfo: expose the standard
	// Composite Temperature and Critical Warning, not synthetic 0x100+ rows.

	// 只读模式（CriticalWarning bit 3）
	if cw&(1<<3) != 0 {
		attrs = append(attrs, Attr{ID: NVMeReadOnly, Name: "Read_Only_Mode", Raw: 1, Kind: "nvme"})
	}
	return attrs
}

// nvmeUint128At 读取小端 128 位计数器的低/高 64 位；字段越界（截断页）时返回 0。
func nvmeUint128At(data []byte, off int) (low, high uint64) {
	if off+16 > len(data) {
		return 0, 0
	}
	return readNVMeUint128(data, off)
}

// nvmeUint32At 读取小端 32 位字段；字段越界（截断页）时返回 0。
func nvmeUint32At(data []byte, off int) uint64 {
	if off+4 > len(data) {
		return 0
	}
	return uint64(binary.LittleEndian.Uint32(data[off : off+4]))
}

func readNVMeUint128(data []byte, off int) (low, high uint64) {
	return binary.LittleEndian.Uint64(data[off : off+8]), binary.LittleEndian.Uint64(data[off+8 : off+16])
}

func parseNVMeCompositeTemperatureThresholds(data []byte) (warningK, criticalK uint64) {
	// NVMe Identify Controller: WCTEMP at byte 266, CCTEMP at byte 268.
	if len(data) < 270 {
		return 0, 0
	}
	return uint64(binary.LittleEndian.Uint16(data[266:268])), uint64(binary.LittleEndian.Uint16(data[268:270]))
}

// ReadNVMeHealth 读取 NVMe 健康日志并返回统一 Attr 列表。
func ReadNVMeHealth(h windows.Handle) ([]Attr, error) {
	attrs, _, _, err := ReadNVMeHealthWithThresholds(h)
	return attrs, err
}

// ReadNVMeHealthWithThresholds reads the standard health log plus optional
// controller-declared composite-temperature thresholds. Identify failures do
// not discard otherwise valid health data.
func ReadNVMeHealthWithThresholds(h windows.Handle) ([]Attr, uint64, uint64, error) {
	attrs, warningK, criticalK, _, err := ReadNVMeHealthWithThresholdsAndTransport(h)
	return attrs, warningK, criticalK, err
}

// ReadNVMeHealthWithThresholdsAndTransport reads standard NVMe health data
// through the protocol-command path first, then the standard storage-property
// query path. The latter helps controllers that expose the log through Windows
// storage properties but reject direct protocol commands.
func ReadNVMeHealthWithThresholdsAndTransport(h windows.Handle) ([]Attr, uint64, uint64, string, error) {
	data, err := issueNVMeGetLogPage(h, NVMeLogID_SMART_Health)
	transport := "NVMe protocol command"
	if err != nil {
		protocolErr := err
		data, err = issueNVMeHealthLogPropertyQuery(h)
		transport = "NVMe storage property query"
		if err != nil {
			return nil, 0, 0, "", fmt.Errorf("NVMe protocol command: %v; storage property query: %w", protocolErr, err)
		}
	}
	attrs := parseNVMeHealthLog(data)
	identify, identifyErr := issueNVMeIdentifyController(h)
	if identifyErr != nil {
		return attrs, 0, 0, transport, nil
	}
	warningK, criticalK := parseNVMeCompositeTemperatureThresholds(identify)
	if warningK > 0 {
		attrs = append(attrs, Attr{ID: NVMeWarningCompositeTempThreshold, Name: "Warning_Composite_Temperature_Threshold_Kelvin", Raw: warningK, Kind: "nvme"})
	}
	if criticalK > 0 {
		attrs = append(attrs, Attr{ID: NVMeCriticalCompositeTempThreshold, Name: "Critical_Composite_Temperature_Threshold_Kelvin", Raw: criticalK, Kind: "nvme"})
	}
	return attrs, warningK, criticalK, transport, nil
}

// NVMeIdentify 通过 IOCTL_STORAGE_QUERY_PROPERTY 获取 NVMe 型号/序列号（更可靠）。
// 这里返回由 enum.go 调用方填充，本文件只负责健康日志。
