package smart

import (
	"encoding/binary"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// ===== NVMe 结构体（NVMe Base Spec 1.4/2.0）=====

// STORAGE_PROTOCOL_COMMAND 是 IOCTL_STORAGE_PROTOCOL_COMMAND 的输入/输出缓冲区。
// 布局（MSDN）：
//   Offset  Size  Field
//   0x00    4     Length（结构长度）
//   0x04    4     ProtocolSpecific（协议类型：NVMe = 0x01）
//   0x08    4     TransferLength（数据缓冲区长度）
//   0x0C    4     ExpectedTransferLength
//   0x10    4     ProtocolSpecificData（NVMe: Get Log Page 的 DWORD0）
//   0x14    2     ProtocolSpecificData2
//   0x16    2     TimeoutValue（秒）
//   0x18    4     dwReserved
//   then: ReturnStatus(4) + ErrorInfoLength(4) + ErrorInfoBuffer + DataBuffer
//
// 为简化，我们使用「命令 + 数据缓冲区」拼接方式。

const (
	STORAGE_PROTOCOL_TYPE_NVMe = 0x01
	NVMeGetLogPage             = 0x02
	NVMeLogID_SMART_Health     = 0x02
)

// buildNVMeGetLogPage 构造 Get Log Page 命令缓冲区。
// 返回 [命令头][数据缓冲区] 拼接后的字节。
func buildNVMeGetLogPage(logID uint8, lenBytes int) []byte {
	// 命令头 40 字节（STORAGE_PROTOCOL_COMMAND 简化版）
	hdr := make([]byte, 40)
	binary.LittleEndian.PutUint32(hdr[0x00:0x04], uint32(40)) // Length
	binary.LittleEndian.PutUint32(hdr[0x04:0x08], STORAGE_PROTOCOL_TYPE_NVMe)
	binary.LittleEndian.PutUint32(hdr[0x08:0x0C], uint32(lenBytes)) // TransferLength
	binary.LittleEndian.PutUint32(hdr[0x0C:0x10], uint32(lenBytes)) // ExpectedTransferLength
	// ProtocolSpecificData: Get Log Page DWORD0 = (NUMDL << 16) | (LID << 8) | (LSP << 7) | RAE
	// 简化：NUMDL = (lenBytes/4 - 1) 低 16 位，LID = logID
	dw0 := (uint32(lenBytes/4-1) & 0xFFFF) << 16
	dw0 |= uint32(logID) << 8
	binary.LittleEndian.PutUint32(hdr[0x10:0x14], dw0)
	// Timeout 10s
	binary.LittleEndian.PutUint16(hdr[0x16:0x18], 10)

	data := make([]byte, lenBytes)
	return append(hdr, data...)
}

// issueNVMeGetLogPage 在已打开的 NVMe 设备上发 Get Log Page 并返回数据缓冲区。
func issueNVMeGetLogPage(h windows.Handle, logID uint8) ([]byte, error) {
	buf := buildNVMeGetLogPage(logID, 512)
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
	if bytesReturned < uint32(40+512) {
		return nil, fmt.Errorf("NVMe 返回过短: got=%d", bytesReturned)
	}
	return outBuf[40:], nil
}

// parseNVMeHealthLog 解析 NVMe SMART/Health Information Log Page（512 字节）。
// 布局（NVMe Base Spec Figure 207）：
//   Offset  Size  Field
//   0x00    1     CriticalWarning（位 0=可用备用不足,1=温度越限,2=可靠性降级,3=只读,4=易失备份失败）
//   0x01    2     Temperature（开尔文，uint16）
//   0x03    1     AvailableSpare（%）
//   0x04    1     AvailableSpareThreshold（%）
//   0x05    1     PercentageUsed（%）
//   0x06    1     EnduranceGroupCriticalWarningSummary
//   0x08    8     DataUnitsRead（1000 * 512B 单位）
//   0x10    8     DataUnitsWritten
//   0x18    8     HostReadCommands
//   0x20    8     HostWriteCommands
//   0x28    8     ControllerBusyTime（分钟）
//   0x30    8     PowerCycles
//   0x38    8     PowerOnHours
//   0x40    8     UnsafeShutdowns
//   0x48    8     MediaErrors（= 用户说的 0E "介质与数据完整性错误计数"）
//   0x50    8     NumErrorInfoLogEntries
//   0x58    4     WarningCompTempTime（分钟）
//   0x5C    4     CriticalCompTempTime（分钟）
//   0x60..  8*8   SensorTemperature[8]（开尔文）
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

	attrs = append(attrs, Attr{ID: NVMeCriticalWarning, Name: "Critical_Warning", Raw: uint64(cw), Kind: "nvme"})
	attrs = append(attrs, Attr{ID: NVMeTemperature, Name: "Temperature_Kelvin", Raw: uint64(tempK), Kind: "nvme"})
	attrs = append(attrs, Attr{ID: NVMeAvailableSpare, Name: "Available_Spare_Pct", Raw: uint64(spare), Kind: "nvme"})
	attrs = append(attrs, Attr{ID: NVMeAvailSpareThresh, Name: "Available_Spare_Threshold", Raw: uint64(spareThresh), Kind: "nvme"})
	attrs = append(attrs, Attr{ID: NVMePercentUsed, Name: "Percentage_Used", Raw: uint64(pctUsed), Kind: "nvme"})
	attrs = append(attrs, Attr{ID: NVMeMediaErrors, Name: "Media_Data_Integrity_Errors", Raw: mediaErrors, Kind: "nvme"})

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
