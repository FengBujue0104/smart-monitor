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
	ID     int    // ATA 属性 ID（NVMe 字段用伪 ID，见下方 NVMe* 常量）
	Name   string // 属性名
	Raw    uint64 // 原始值（已解码为可用数值）
	Value  int    // 归一化当前值（ATA: 1-253；NVMe 不使用）
	Worst  int    // 归一化历史最差值（ATA）
	Thresh int    // 阈值（ATA）
	Kind   string // "ata" / "nvme"
}

// NVMe 字段伪 ID（用于 Attr.ID，避免与 ATA 冲突）
const (
	NVMeCriticalWarning  = 0xF0
	NVMeTemperature      = 0xF1
	NVMeAvailableSpare   = 0xF2
	NVMeAvailSpareThresh = 0xF3
	NVMePercentUsed      = 0xF4
	NVMeMediaErrors      = 0xF5 // "Media and Data Integrity Errors"
	NVMeReadOnly         = 0xF6
	NVMeDataUnitsRead    = 0xF7
	NVMeDataUnitsWritten = 0xF8
	NVMePowerCycles      = 0xF9
	NVMePowerOnHours     = 0xFA
	NVMeUnsafeShutdowns  = 0xFB
	NVMeErrorInfoEntries = 0xFC
	NVMeWarningTempTime  = 0xFD
	NVMeCriticalTempTime = 0xFE
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
	// ATA 特有
	SmartStatusKnown  bool // 是否成功读取 ATA SMART overall status
	SmartStatusPassed bool // ATA SMART overall status 是否通过
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
