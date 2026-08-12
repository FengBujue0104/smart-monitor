package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
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
		if err := ui.RunReportWithStatus(nil, nil, "⚠ 未以管理员身份运行，无法读取硬盘 S.M.A.R.T 数据。请退出后右键选择“以管理员身份运行”。"); err != nil {
			log.Printf("UI error: %v", err)
		}
		return
	}

	// 日志默认写 exe 所在目录；失败（如 exe 放在 Program Files 只读目录）时
	// 回退到 %TEMP%\smonitor.log，保证诊断日志总能落盘。
	logFile, err := openLogFile()
	if err == nil {
		defer logFile.Close()
		log.SetOutput(logFile)
	}

	// 每次扫描均优先使用 IOCTL，并按同一套规则合并 WMI 回退；这让首次
	// 扫描和 GUI 的“重新扫描”在 USB/RAID 控制器上的结果保持一致。
	disks, err := smart.DiscoverWithFallback()
	if err != nil {
		if uiErr := ui.RunReportWithStatus(nil, nil, fmt.Sprintf("❌ 扫描失败：枚举磁盘失败: %v", err)); uiErr != nil {
			log.Printf("UI error: %v", uiErr)
		}
		return
	}
	if len(disks) == 0 {
		if err := ui.RunReportWithStatus(nil, nil, "⚠ 未找到任何物理磁盘。"); err != nil {
			log.Printf("UI error: %v", err)
		}
		return
	}

	violations := health.Evaluate(disks)
	if err := ui.RunReport(disks, violations); err != nil {
		log.Printf("UI error: %v", err)
		os.Exit(3)
	}
}

// openLogFile 优先在当前目录创建日志；不可写时回退到系统临时目录。
// 返回的 *os.File 为 nil 时调用方应跳过日志输出（静默降级）。
func openLogFile() (*os.File, error) {
	f, err := os.OpenFile("smonitor.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err == nil {
		return f, nil
	}
	tmp := filepath.Join(os.TempDir(), "smonitor.log")
	return os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
}

// isAdmin checks the process token directly. Opening a physical drive is not a
// reliable privilege check: a healthy elevated process may still be denied by
// a controller, policy, or an offline disk.
func isAdmin() bool {
	return windows.GetCurrentProcessToken().IsElevated()
}
