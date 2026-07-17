package smart

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"runtime"

	"golang.org/x/sys/windows"
)

// ===== ATA 命令 =====
const (
	ATA_CMD_SMART           = 0xB0
	ATA_CMD_IDENTIFY_DEVICE = 0xEC
)

const (
	SMART_READ_DATA       = 0xD0
	SMART_READ_THRESHOLDS = 0xD1
	SMART_RETURN_STATUS   = 0xDA
)

// ===== SMART_RCV_DRIVE_DATA / SEND_DRIVE_DATA（ntdddisk.h）=====
// 这是 Windows 原生 SMART 接口，兼容性最好。
// IOCTL = CTL_CODE(IOCTL_DISK_BASE=7, 0x0020, METHOD_BUFFERED, FILE_READ_ACCESS|FILE_WRITE_ACCESS)
//
//	= (7<<16)|(3<<14)|(0x20<<2)|0 = 0x7C088
const IOCTL_SMART_RCV_DRIVE_DATA = 0x7C088
const IOCTL_SMART_SEND_DRIVE_DATA = 0x7C08C

// SENDCMDINPARAMS 输入结构（ntdddisk.h）。
// 布局（小端）：
//
//	Offset  Size  Field
//	0x00    4     cBufferSize（数据缓冲区大小，通常 512）
//	0x04    1     irDriveRegs.bFeaturesReg
//	0x05    1     irDriveRegs.bSectorCountReg
//	0x06    1     irDriveRegs.bSectorNumberReg
//	0x07    1     irDriveRegs.bCylLowReg
//	0x08    1     irDriveRegs.bCylHighReg
//	0x09    1     irDriveRegs.bDriveHeadReg
//	0x0A    1     irDriveRegs.bCommandReg
//	0x0B    1     irDriveRegs.bReserved
//	0x0C    1     bDriveNumber
//	0x0D    3     bReserved[3]
//	0x10    16    dwReserved[4]
//	0x20    1     bBuffer[1]（变长，实际紧跟头后）
const sendCmdParamsSize = 0x20 // 32

// buildSmartCmd 构造 SMART_RCV_DRIVE_DATA / SEND_DRIVE_DATA 的输入缓冲区。
// cmd: ATA 命令（0xB0 SMART / 0xEC IDENTIFY）
// sub: SMART 子命令（0xD0 READ_DATA / 0xD1 READ_THRESHOLDS），IDENTIFY 填 0
// driveNum: 物理驱动器编号（0,1,2...）
// buf: 数据缓冲区（长度 >= 512）
func buildSmartCmd(cmd, sub, driveNum byte, buf []byte) []byte {
	n := sendCmdParamsSize + len(buf)
	raw := make([]byte, n)

	// cBufferSize
	binary.LittleEndian.PutUint32(raw[0x00:0x04], uint32(len(buf)))

	// IDEREGS（8 字节，偏移 0x04）
	reg := raw[0x04:0x0C]
	switch cmd {
	case ATA_CMD_SMART:
		reg[0] = sub  // bFeaturesReg = 子命令
		reg[1] = 0    // bSectorCountReg
		reg[2] = 0x4F // bSectorNumberReg（LBA Low，SMART 签名）
		reg[3] = 0xC2 // bCylLowReg（LBA Mid，SMART 签名）
		reg[4] = 0    // bCylHighReg
		reg[5] = 0xA0 // bDriveHeadReg（Device = master）
		reg[6] = cmd  // bCommandReg
		reg[7] = 0    // bReserved
	case ATA_CMD_IDENTIFY_DEVICE:
		reg[0] = 0
		reg[1] = 0
		reg[2] = 0
		reg[3] = 0
		reg[4] = 0
		reg[5] = 0xA0
		reg[6] = cmd
		reg[7] = 0
	}

	// bDriveNumber
	raw[0x0C] = driveNum

	// 拷贝数据缓冲区
	copy(raw[sendCmdParamsSize:], buf)
	return raw
}

// buildSMARTReturnStatus builds an ATA_PASS_THROUGH_EX request without a data buffer.
func buildSMARTReturnStatus() []byte {
	headerSize, taskFileOffset := 48, 40 // x64 layout
	if runtime.GOARCH == "386" {
		headerSize, taskFileOffset = 40, 32
	}
	buf := make([]byte, headerSize)
	binary.LittleEndian.PutUint16(buf[0:2], uint16(headerSize))
	binary.LittleEndian.PutUint16(buf[2:4], 0x0001) // ATA_FLAGS_DRDY_REQUIRED
	binary.LittleEndian.PutUint32(buf[12:16], 10)   // TimeOutValue
	// PreviousTaskFile is zero. CurrentTaskFile is the ATA task-file register set.
	tf := buf[taskFileOffset:]
	tf[0] = SMART_RETURN_STATUS // Features: SMART RETURN STATUS subcommand
	tf[2] = 0x4F                // LBA low signature
	tf[3] = 0xC2                // LBA mid signature
	tf[5] = 0xA0                // Device
	tf[6] = ATA_CMD_SMART
	return buf
}

func parseSMARTReturnStatus(taskFile []byte) (bool, error) {
	if len(taskFile) < 5 {
		return false, fmt.Errorf("SMART RETURN STATUS task file too short")
	}
	// ATA specifies 0x4F/0xC2 for pass and 0xF4/0x2C when a threshold is exceeded.
	if taskFile[3] == 0x4F && taskFile[4] == 0xC2 {
		return true, nil
	}
	if taskFile[3] == 0xF4 && taskFile[4] == 0x2C {
		return false, nil
	}
	return false, fmt.Errorf("unknown SMART RETURN STATUS signature: 0x%02X/0x%02X", taskFile[3], taskFile[4])
}

// ReadSMARTOverallStatus reads ATA SMART RETURN STATUS through ATA pass-through.
func ReadSMARTOverallStatus(h windows.Handle) (bool, error) {
	buf := buildSMARTReturnStatus()
	var returned uint32
	if err := windows.DeviceIoControl(h, IOCTL_ATA_PASS_THROUGH_EX,
		&buf[0], uint32(len(buf)), &buf[0], uint32(len(buf)), &returned, nil); err != nil {
		return false, fmt.Errorf("ATA SMART RETURN STATUS: %w", err)
	}
	if runtime.GOARCH == "386" {
		return parseSMARTReturnStatus(buf[32:40])
	}
	return parseSMARTReturnStatus(buf[40:48])
}

// issueSmartCommand 通过 SMART_RCV_DRIVE_DATA 发命令并返回数据缓冲区。
func issueSmartCommand(h windows.Handle, cmd, sub, driveNum byte) ([]byte, error) {
	buf := make([]byte, 512)
	pkt := buildSmartCmd(cmd, sub, driveNum, buf)

	outBuf := make([]byte, len(pkt))
	var bytesReturned uint32

	err := windows.DeviceIoControl(
		h,
		IOCTL_SMART_RCV_DRIVE_DATA,
		&pkt[0],
		uint32(len(pkt)),
		&outBuf[0],
		uint32(len(outBuf)),
		&bytesReturned,
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("SMART_RCV_DRIVE_DATA cmd=0x%02X err=%w", cmd, err)
	}
	// 输出结构：SENDCMDOUTPARAMS = cBufferSize(4) + DriverStatus{bDriverError,bIDEError,bReserved[2],dwReserved[2]}(12) = 16 字节头
	// 然后紧跟 512 字节数据缓冲区。故总返回 = 16 + 512 = 528。
	const outHdrSize = 16
	if bytesReturned < uint32(outHdrSize+12) {
		return nil, fmt.Errorf("SMART_RCV 返回过短: got=%d", bytesReturned)
	}
	dataLen := bytesReturned - outHdrSize
	if dataLen > 512 {
		dataLen = 512
	}
	data := make([]byte, 512)
	copy(data, outBuf[outHdrSize:outHdrSize+dataLen])
	return data, nil
}

// ReadIdentify 读取 ATA IDENTIFY DEVICE（512 字节）。
func ReadIdentify(h windows.Handle, driveNum byte) (model, serial, firmware string, err error) {
	data, err := issueSmartCommand(h, ATA_CMD_IDENTIFY_DEVICE, 0, driveNum)
	if err != nil {
		return "", "", "", err
	}
	firmware = string(bytes.TrimSpace(swapUTF16Bytes(data[23*2 : 27*2])))
	serial = string(bytes.TrimSpace(swapUTF16Bytes(data[10*2 : 20*2])))
	model = string(bytes.TrimSpace(swapUTF16Bytes(data[27*2 : 47*2])))
	return model, serial, firmware, nil
}

// ReadSMARTData 读取 SMART 属性表。
func ReadSMARTData(h windows.Handle, driveNum byte) ([]Attr, bool, error) {
	data, err := issueSmartCommand(h, ATA_CMD_SMART, SMART_READ_DATA, driveNum)
	if err != nil {
		return nil, false, err
	}
	return parseSMARTData(data), true, nil
}

func parseSMARTData(data []byte) []Attr {
	if len(data) < 2 {
		return nil
	}
	base := smartTableBase(data)
	var attrs []Attr
	for i := 0; i < 30; i++ {
		// 标准 SMART 属性表：2 字节版本头 + 30 × 12 字节属性。
		// 某些 AHCI 驱动不带版本表头（从偏移 0 开始），需启发式识别：
		//   若 data[0] 为 0 且 data[2] 是有效属性 ID（1..0xC7），则表头为 2 字节；
		//   否则可能从偏移 0 开始。
		off := base + i*12
		if off+12 > len(data) {
			break
		}
		id := int(data[off])
		if id == 0 {
			continue
		}
		value := int(data[off+3])
		worst := int(data[off+4])
		// SMART raw 值为 48 位（6 字节），但某些厂商打包为 32 位。
		// 低 48 位取自 data[off+5..off+11)，小端。
		var raw uint64
		if off+11 <= len(data) {
			raw = uint64(data[off+5]) | uint64(data[off+6])<<8 | uint64(data[off+7])<<16 |
				uint64(data[off+8])<<24 | uint64(data[off+9])<<32 | uint64(data[off+10])<<48
		}
		attrs = append(attrs, Attr{
			ID:    id,
			Name:  ATAAttrName(id),
			Raw:   raw,
			Value: value,
			Worst: worst,
			Kind:  "ata",
		})
	}
	return attrs
}

// smartTableBase chooses the standard two-byte header unless the complete
// table only makes sense when interpreted as a headerless vendor response.
func smartTableBase(data []byte) int {
	count := func(base int) int {
		count := 0
		for i := 0; i < 30; i++ {
			off := base + i*12
			if off+12 > len(data) {
				break
			}
			id, value, worst := data[off], data[off+3], data[off+4]
			if id != 0 && value <= 253 && worst <= 253 {
				count++
			}
		}
		return count
	}
	if count(2) > 0 || count(0) == 0 {
		return 2
	}
	return 0
}

// ReadSMARTThresholds 读取属性阈值页。
func ReadSMARTThresholds(h windows.Handle, driveNum byte) (map[int]int, error) {
	data, err := issueSmartCommand(h, ATA_CMD_SMART, SMART_READ_THRESHOLDS, driveNum)
	if err != nil {
		return nil, err
	}
	m := map[int]int{}
	for i := 0; i < 30; i++ {
		off := 2 + i*12
		if off+12 > len(data) {
			break
		}
		id := int(data[off])
		if id == 0 {
			continue
		}
		thresh := int(data[off+1])
		m[id] = thresh
	}
	return m, nil
}

// swapUTF16Bytes 对每 2 字节做 byte-swap（ATA IDENTIFY 双字节反序）
func swapUTF16Bytes(b []byte) []byte {
	out := make([]byte, len(b))
	for i := 0; i+1 < len(b); i += 2 {
		out[i] = b[i+1]
		out[i+1] = b[i]
	}
	return out
}
