package main

import (
	"fmt"
	"log"
	"os"
	"runtime"

	"github.com/lxn/walk"
	"golang.org/x/sys/windows"
	"smonitor/health"
	"smonitor/smart"
	"smonitor/ui"
)

func main() {
	// 主线程必须锁定（Win32 GUI 要求），并初始化 COM（STA）以避免 ToolTip TTM_ADDTOOL 失败。
	runtime.LockOSThread()
	// STA：多线程公寓对 ToolTip/CommonDialog 更稳定
	_ = windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED|windows.COINIT_DISABLE_OLE1DDE)
	defer windows.CoUninitialize()

	// 设置 walk 应用元数据（影响注册表/设置存储路径，也触发内部初始化）
	walk.App().SetOrganizationName("smonitor")
	walk.App().SetProductName("S.M.A.R.T 健康检查工具")

	// 检查管理员权限（SMART IOCTL 必需）
	if !isAdmin() {
		walk.MsgBox(nil, "需要管理员权限",
			"本工具需要管理员权限才能读取硬盘 S.M.A.R.T 数据。\n请右键 → 以管理员身份运行。",
			walk.MsgBoxIconWarning)
		os.Exit(1)
	}

	logFile, err := os.OpenFile("smonitor.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err == nil {
		defer logFile.Close()
		log.SetOutput(logFile)
	}

	// 优先使用 IOCTL：它能区分 ATA/NVMe，并按 PhysicalDrive 建立一对一关联。
	// WMI 仅作为控制器不支持 IOCTL 时的兼容回退。
	disks, err := smart.Discover()
	if err != nil || len(disks) == 0 {
		disks, err = smart.DiscoverWMI()
		if err != nil {
			walk.MsgBox(nil, "扫描失败", fmt.Sprintf("枚举磁盘失败: %v", err), walk.MsgBoxIconError)
			os.Exit(2)
		}
	} else {
		// IOCTL 可能无法打开 RAID、USB 桥接或权限受限的某一块物理盘。始终
		// 合并 WMI 的枚举结果，既补齐缺失属性，也补回主路径完全未发现的磁盘。
		if fallback, wmiErr := smart.DiscoverWMI(); wmiErr == nil {
			disks = mergeFallbackDisks(disks, fallback)
		}
	}
	if len(disks) == 0 {
		walk.MsgBox(nil, "未发现磁盘", "未找到任何物理磁盘。", walk.MsgBoxIconInformation)
		os.Exit(0)
	}

	violations := health.Evaluate(disks)
	if err := ui.RunReport(disks, violations); err != nil {
		log.Printf("UI error: %v", err)
		os.Exit(3)
	}
}

func mergeFallbackDisks(primary, fallback []smart.Disk) []smart.Disk {
	byIndex := make(map[int]smart.Disk, len(fallback))
	for _, d := range fallback {
		byIndex[d.Index] = d
	}
	seen := make(map[int]bool, len(primary))
	result := make([]smart.Disk, 0, len(primary)+len(fallback))
	for _, d := range primary {
		seen[d.Index] = true
		if f, ok := byIndex[d.Index]; ok && f.Kind == d.Kind {
			primaryCorrupt := d.Kind == smart.KindATA && d.SMARTChecksumKnown && !d.SMARTChecksumValid
			fallbackCanReplace := len(d.Attrs) == 0 || (primaryCorrupt && f.SMARTChecksumKnown && f.SMARTChecksumValid)
			if fallbackCanReplace && len(f.Attrs) > 0 {
				d.Attrs = f.Attrs
				// WMI 数据页带有完整页校验时，可明确用它替换损坏的 IOCTL 页。
				if f.SMARTChecksumKnown {
					d.SMARTChecksumKnown = true
					d.SMARTChecksumValid = f.SMARTChecksumValid
				}
			}
			// ATA pass-through 常能读取属性但不能返回 SMART RETURN STATUS。
			// 此时用 WMI 的状态补齐；已由主路径成功读取的状态绝不覆盖。
			if !d.SmartStatusKnown && f.SmartStatusKnown {
				d.SmartStatusKnown = true
				d.SmartStatusPassed = f.SmartStatusPassed
			}
		}
		result = append(result, d)
	}
	for _, d := range fallback {
		if !seen[d.Index] {
			result = append(result, d)
		}
	}
	return result
}

// isAdmin checks the process token directly. Opening a physical drive is not a
// reliable privilege check: a healthy elevated process may still be denied by
// a controller, policy, or an offline disk.
func isAdmin() bool {
	return windows.GetCurrentProcessToken().IsElevated()
}
