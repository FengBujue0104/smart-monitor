package smart

import "time"

// DiskKind 表示磁盘类型
type DiskKind string

const (
	KindATA  DiskKind = "ATA"
	KindNVMe DiskKind = "NVMe"
)

// Attr 是统一后的 SMART 属性（ATA 与 NVMe 共用）
type Attr struct {
	ID      int    // ATA 属性 ID（NVMe 字段用伪 ID，见下方 NVMe* 常量）
	Name    string // 属性名
	Flags   uint16 // ATA SMART 属性标志位（NVMe 为 0）
	Raw     uint64 // 原始值（已解码为可用数值）
	RawHigh uint64 // 128 位 NVMe 计数的高 64 位，ATA/普通字段为 0
	Value   int    // 归一化当前值（ATA: 1-253；NVMe 不使用）
	Worst   int    // 归一化历史最差值（ATA）
	Thresh  int    // 阈值（ATA）
	Kind    string // "ata" / "nvme"
}

// NVMe 字段伪 ID（用于 Attr.ID，避免与 ATA 冲突）
const (
	NVMeCriticalWarning    = 0xF0
	NVMeTemperature        = 0xF1
	NVMeAvailableSpare     = 0xF2
	NVMeAvailSpareThresh   = 0xF3
	NVMePercentUsed        = 0xF4
	NVMeMediaErrors        = 0xF5 // "Media and Data Integrity Errors"
	NVMeReadOnly           = 0xF6
	NVMeDataUnitsRead      = 0xF7
	NVMeDataUnitsWritten   = 0xF8
	NVMePowerCycles        = 0xF9
	NVMePowerOnHours       = 0xFA
	NVMeUnsafeShutdowns    = 0xFB
	NVMeErrorInfoEntries   = 0xFC
	NVMeWarningTempTime    = 0xFD
	NVMeCriticalTempTime   = 0xFE
	NVMeTemperatureSensor1 = 0x100
	NVMeTemperatureSensor2 = 0x101
	NVMeTemperatureSensor3 = 0x102
	NVMeTemperatureSensor4 = 0x103
	NVMeTemperatureSensor5 = 0x104
	NVMeTemperatureSensor6 = 0x105
	NVMeTemperatureSensor7 = 0x106
	NVMeTemperatureSensor8 = 0x107
)

// Disk 是一块物理磁盘的 SMART 汇总
type Disk struct {
	Index    int      // PhysicalDrive 编号
	Path     string   // \\.\PhysicalDriveN
	Kind     DiskKind // ATA / NVMe
	Model    string
	Serial   string
	Firmware string
	SizeGB   float64
	Attrs    []Attr
	// 设备报告的整体 SMART 状态（ATA pass-through 或 WMI 预测状态）。
	SmartStatusKnown  bool // 是否成功读取整体 SMART status
	SmartStatusPassed bool // 整体 SMART status 是否通过
	// ATA 特有
	SMARTChecksumKnown bool // 是否检查了 ATA SMART 数据 checksum
	SMARTChecksumValid bool // ATA SMART 数据 checksum 是否有效
	// NVMe 特有
	NVMeCriticalSpareBelow bool
	NVMeReadOnlyMode       bool
}

// Report 是最终生成的文本报表
type Report struct {
	GeneratedAt time.Time
	Hostname    string
	Disks       []Disk
	Violations  []Violation
}

// Violation 表示一项阈值违规
type Violation struct {
	DiskIndex int
	DiskModel string
	AttrID    int
	AttrName  string
	Current   string // 当前值（可读）
	Limit     string // 阈值（可读)
	Severity  string // "critical" / "warning"
}
