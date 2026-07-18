package smart

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"runtime"
	"unsafe"

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

const (
	scsiIOCTLDataIn            = 1
	satPassThrough16           = 0x85
	satProtocolPIODataIn       = 4
	satFlagTransferSectorCount = 0x0E // T_DIR | BYT_BLOK | T_LENGTH=sector count
)

// scsiPassThroughDirect matches SCSI_PASS_THROUGH_DIRECT on supported Windows
// architectures. The project is packaged for amd64, and unsafe.Sizeof keeps
// Length correct if it is built for another Windows architecture.
type scsiPassThroughDirect struct {
	Length             uint16
	ScsiStatus         byte
	PathID             byte
	TargetID           byte
	Lun                byte
	CdbLength          byte
	SenseInfoLength    byte
	DataIn             byte
	DataTransferLength uint32
	TimeOutValue       uint32
	DataBuffer         *byte
	SenseInfoOffset    uint32
	Cdb                [16]byte
}

type scsiPassThroughDirectWithSense struct {
	request scsiPassThroughDirect
	sense   [32]byte
}

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
		reg[0] = sub // bFeaturesReg = 子命令
		reg[1] = 1   // bSectorCountReg：SMART data/thresholds 各传输一个扇区
		reg[2] = 0   // bSectorNumberReg（LBA Low）
		// ATA SMART 的固定签名在 LBA Mid/High（即 Cylinder Low/High），
		// 不能写入 SectorNumber/CylinderLow；否则多数控制器会拒绝命令。
		reg[3] = 0x4F // bCylLowReg（LBA Mid，SMART 签名低字节）
		reg[4] = 0xC2 // bCylHighReg（LBA High，SMART 签名高字节）
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
	tf[3] = 0x4F                // LBA Mid / Cylinder Low SMART signature
	tf[4] = 0xC2                // LBA High / Cylinder High SMART signature
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
		taskFile, err := smartStatusTaskFile(buf, returned, 32)
		if err != nil {
			return false, err
		}
		return parseSMARTReturnStatus(taskFile)
	}
	taskFile, err := smartStatusTaskFile(buf, returned, 40)
	if err != nil {
		return false, err
	}
	return parseSMARTReturnStatus(taskFile)
}

// smartStatusTaskFile rejects responses which did not include the returned
// task file. Without this check the input pass signature could be mistaken for
// a successful SMART status after a controller returns a short response.
func smartStatusTaskFile(buf []byte, returned uint32, offset int) ([]byte, error) {
	const taskFileSize = 8
	if offset < 0 || offset+taskFileSize > len(buf) || returned < uint32(offset+taskFileSize) {
		return nil, fmt.Errorf("ATA SMART RETURN STATUS response too short: got=%d", returned)
	}
	return buf[offset : offset+taskFileSize], nil
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
	if bytesReturned < uint32(outHdrSize+512) {
		return nil, fmt.Errorf("SMART_RCV 返回过短: got=%d", bytesReturned)
	}
	if err := parseSMARTDriverStatus(outBuf); err != nil {
		return nil, fmt.Errorf("SMART_RCV_DRIVE_DATA cmd=0x%02X: %w", cmd, err)
	}
	dataLen := bytesReturned - outHdrSize
	if dataLen > 512 {
		dataLen = 512
	}
	data := make([]byte, 512)
	copy(data, outBuf[outHdrSize:outHdrSize+dataLen])
	return data, nil
}

func parseSMARTDriverStatus(out []byte) error {
	if len(out) < 6 {
		return fmt.Errorf("output header too short")
	}
	if out[4] != 0 || out[5] != 0 {
		return fmt.Errorf("driver error=0x%02X ide error=0x%02X", out[4], out[5])
	}
	return nil
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
	attrs, ok, _, err := ReadSMARTDataDetailed(h, driveNum)
	return attrs, ok, err
}

func ReadSMARTDataDetailed(h windows.Handle, driveNum byte) ([]Attr, bool, bool, error) {
	data, err := issueSmartCommand(h, ATA_CMD_SMART, SMART_READ_DATA, driveNum)
	if err != nil {
		return nil, false, false, err
	}
	return parseSMARTData(data), true, smartChecksumValid(data), nil
}

// buildSATSMARTReadCDB creates an ATA PASS-THROUGH(16) SMART data command.
// SAT (SCSI / ATA Translation) is used by a large class of USB-to-SATA bridges.
func buildSATSMARTReadCDB(subcommand byte) [16]byte {
	var cdb [16]byte
	cdb[0] = satPassThrough16
	cdb[1] = satProtocolPIODataIn << 1
	cdb[2] = satFlagTransferSectorCount
	cdb[4] = subcommand // ATA Features: SMART subcommand
	cdb[6] = 1          // ATA Sector Count: one 512-byte SMART page
	cdb[10] = 0x4F      // ATA LBA Mid SMART signature
	cdb[12] = 0xC2      // ATA LBA High SMART signature
	cdb[13] = 0xA0      // ATA Device
	cdb[14] = ATA_CMD_SMART
	return cdb
}

func issueSATSMARTRead(h windows.Handle, subcommand byte) ([]byte, error) {
	data := make([]byte, 512)
	packet := scsiPassThroughDirectWithSense{}
	req := &packet.request
	req.Length = uint16(unsafe.Sizeof(*req))
	req.CdbLength = uint8(len(req.Cdb))
	req.SenseInfoLength = uint8(len(packet.sense))
	req.DataIn = scsiIOCTLDataIn
	req.DataTransferLength = uint32(len(data))
	req.TimeOutValue = 10
	req.DataBuffer = &data[0]
	req.SenseInfoOffset = uint32(unsafe.Offsetof(packet.sense))
	req.Cdb = buildSATSMARTReadCDB(subcommand)

	var returned uint32
	err := windows.DeviceIoControl(
		h,
		IOCTL_SCSI_PASS_THROUGH_DIRECT,
		(*byte)(unsafe.Pointer(&packet)),
		uint32(unsafe.Sizeof(packet)),
		(*byte)(unsafe.Pointer(&packet)),
		uint32(unsafe.Sizeof(packet)),
		&returned,
		nil,
	)
	runtime.KeepAlive(data)
	if err != nil {
		return nil, fmt.Errorf("SAT ATA PASS-THROUGH(16) SMART 0x%02X: %w", subcommand, err)
	}
	if req.ScsiStatus != 0 {
		senseLen := int(req.SenseInfoLength)
		if senseLen > len(packet.sense) {
			senseLen = len(packet.sense)
		}
		return nil, fmt.Errorf("SAT ATA PASS-THROUGH(16) SMART 0x%02X SCSI status=0x%02X sense=% X", subcommand, req.ScsiStatus, packet.sense[:senseLen])
	}
	return data, nil
}

// ReadSMARTDataSATDetailed reads the standard ATA SMART data page through a
// SAT-capable SCSI or USB bridge. It is a read-only fallback for controllers
// that reject the legacy SMART_RCV_DRIVE_DATA IOCTL.
func ReadSMARTDataSATDetailed(h windows.Handle) ([]Attr, bool, bool, error) {
	data, err := issueSATSMARTRead(h, SMART_READ_DATA)
	if err != nil {
		return nil, false, false, err
	}
	return parseSMARTData(data), true, smartChecksumValid(data), nil
}

func smartChecksumValid(data []byte) bool {
	if len(data) < 512 {
		return false
	}
	var sum byte
	for _, b := range data[:512] {
		sum += b
	}
	return sum == 0
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
		flags := uint16(data[off+1]) | uint16(data[off+2])<<8
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
			Flags: flags,
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
	thresholds, checksumValid, err := ReadSMARTThresholdsDetailed(h, driveNum)
	if err != nil {
		return nil, err
	}
	if !checksumValid {
		return nil, fmt.Errorf("ATA SMART threshold checksum invalid")
	}
	return thresholds, nil
}

// ReadSMARTThresholdsDetailed reads the ATA threshold page and reports whether
// its 512-byte SMART checksum is valid.
func ReadSMARTThresholdsDetailed(h windows.Handle, driveNum byte) (map[int]int, bool, error) {
	data, err := issueSmartCommand(h, ATA_CMD_SMART, SMART_READ_THRESHOLDS, driveNum)
	if err != nil {
		return nil, false, err
	}
	return parseSMARTThresholds(data), smartChecksumValid(data), nil
}

// ReadSMARTThresholdsSATDetailed reads the ATA SMART threshold page through
// a SAT-capable SCSI or USB bridge.
func ReadSMARTThresholdsSATDetailed(h windows.Handle) (map[int]int, bool, error) {
	data, err := issueSATSMARTRead(h, SMART_READ_THRESHOLDS)
	if err != nil {
		return nil, false, err
	}
	return parseSMARTThresholds(data), smartChecksumValid(data), nil
}

func parseSMARTThresholds(data []byte) map[int]int {
	m := map[int]int{}
	base := smartTableBase(data)
	for i := 0; i < 30; i++ {
		off := base + i*12
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
	return m
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
