package ui

import "golang.org/x/sys/windows"

var (
	user32DLL = windows.NewLazySystemDLL("user32.dll")
	msgBeep   = user32DLL.NewProc("MessageBeep")
)

// MB_ICONERROR: 系统默认错误提示音（非阻塞）。
const mbIconError = 0x10

// alertBeep 在出现新的严重告警时播放系统错误提示音，提醒用户窗口可能
// 不在前台。仅在状态从“无严重”变为“有严重”时调用一次。
func alertBeep() {
	msgBeep.Call(mbIconError)
}
