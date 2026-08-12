package smart

import (
	"golang.org/x/sys/windows"
)

// IOCTL 控制码 —— 经 CTL_CODE 宏核算验证（见仓库 build 目录 ctlcalc.go 辅助计算脚本）。
// CTL_CODE(dev, func, method, access) = (dev<<16) | (access<<14) | (func<<2) | method
// dev=4 (IOCTL_SCSI_BASE), access=3 (FILE_READ|FILE_WRITE), method=0 (METHOD_BUFFERED)
// => dev<<16 = 0x40000，access<<14 = 0xC000，即前缀 0x4C000，再加上 (func<<2)。
const (
	// ATA passthrough（已验证值）
	IOCTL_ATA_PASS_THROUGH    = 0x4D02C // func 0x040b
	IOCTL_ATA_PASS_THROUGH_EX = 0x4D030 // func 0x040c

	// 注意：SMART_RCV/SEND_DRIVE_DATA 的正确控制码是 0x7C088/0x7C08C（ntdddisk.h，
	// 见 ata.go 的 IOCTL_SMART_RCV_DRIVE_DATA / IOCTL_SMART_SEND_DRIVE_DATA），
	// 不要误用上方 0x4D030/0x4D034（那分别是 ATA_PASS_THROUGH_EX / PASS_THROUGH）。

	// SCSI Miniport
	IOCTL_SCSI_MINIPORT = 0x4D008
	// SCSI pass-through direct. SAT-capable USB/SCSI bridges accept ATA
	// PASS-THROUGH(16) through this IOCTL, which is the standard fallback when
	// SMART_RCV_DRIVE_DATA is unavailable outside a native ATA controller.
	IOCTL_SCSI_PASS_THROUGH_DIRECT = 0x4D014

	// 存储属性查询（DeviceIOControl 官方值）
	IOCTL_STORAGE_QUERY_PROPERTY = 0x002D1400

	// NVMe Get Log Page（Win8.1+/Win10+ 稳定）
	IOCTL_STORAGE_PROTOCOL_COMMAND               = 0x002D9808
	STORAGE_PROTOCOL_SPECIFIC_NVME_ADMIN_COMMAND = 0x01
)

// ATA 命令与 SMART 子命令已移至 ata.go，避免重定义。

// ===== 通用辅助 =====

func toUTF16Ptr(s string) *uint16 {
	p, _ := windows.UTF16PtrFromString(s)
	return p
}

// openDevice 以读写+共享方式打开 \\.\PhysicalDriveN
func openDevice(path string) (windows.Handle, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	return windows.CreateFile(
		p,
		windows.GENERIC_READ|windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		nil,
		windows.OPEN_EXISTING,
		0,
		0,
	)
}
