package main

import (
	"log"
	"os"
	"path/filepath"
	"runtime"

	"github.com/lxn/walk"
	"golang.org/x/sys/windows"
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
		if err := ui.RunReportWithStatus(nil, nil, "未以管理员身份运行，无法读取硬盘 S.M.A.R.T 数据。请退出后右键选择“以管理员身份运行”。"); err != nil {
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

	// 立即显示窗口并后台扫描：USB/RAID 探测可能耗时数秒，同步扫描会让
	// 双击后的窗口长时间不出现。扫描失败/无磁盘也由窗口横幅呈现，无需
	// 启动前的阻塞分支。
	if err := ui.RunReportWithScan(smart.DiscoverWithFallback); err != nil {
		log.Printf("UI error: %v", err)
		os.Exit(3)
	}
}

// 单日/长期运行的日志上限：超过后把旧日志归档为 smonitor.log.old 再重写，
// 避免 smonitor.log 无限增长。
const maxLogBytes = 5 << 20 // 5 MB

// openLogFile 优先在 exe 所在目录创建日志；不可写时回退到系统临时目录。
// 返回的 *os.File 为 nil 时调用方应跳过日志输出（静默降级）。
func openLogFile() (*os.File, error) {
	dir := "."
	if exe, err := os.Executable(); err == nil {
		dir = filepath.Dir(exe)
	}
	if f, err := openLogWithRotation(filepath.Join(dir, "smonitor.log")); err == nil {
		return f, nil
	}
	return openLogWithRotation(filepath.Join(os.TempDir(), "smonitor.log"))
}

// openLogWithRotation 若日志已超过 maxLogBytes 则先归档（smonitor.log.old，
// 覆盖旧的归档），再以追加方式打开。归档失败不阻塞：继续打开原文件。
func openLogWithRotation(path string) (*os.File, error) {
	if fi, err := os.Stat(path); err == nil && fi.Size() > maxLogBytes {
		_ = os.Rename(path, path+".old")
	}
	return os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
}

// isAdmin checks the process token directly. Opening a physical drive is not a
// reliable privilege check: a healthy elevated process may still be denied by
// a controller, policy, or an offline disk.
func isAdmin() bool {
	return windows.GetCurrentProcessToken().IsElevated()
}
