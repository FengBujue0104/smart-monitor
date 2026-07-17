package ui

import (
	"fmt"
	"strings"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
	"golang.org/x/sys/windows"
	"smonitor/health"
)

// showRedAlert 弹出红色告警窗口（独立、置顶、不抢焦点）。
func showRedAlert(vs []health.Violation) {
	var lines []string
	for _, v := range vs {
		mark := "⚠️"
		if v.Severity == "critical" {
			mark = "❌"
		}
		lines = append(lines, fmt.Sprintf("%s [Disk%d] %s: %s (阈值 %s)",
			mark, v.DiskIndex, v.AttrName, v.Current, v.Limit))
	}
	body := strings.Join(lines, "\n")

	var mw *walk.MainWindow
	err := (MainWindow{
		AssignTo: &mw,
		Title:    "⚠️ 硬盘告警",
		Size:     Size{Width: 760, Height: 240},
		Layout:   VBox{Margins: Margins{Left: 12, Top: 12, Right: 12, Bottom: 12}},
		Children: []Widget{
			Label{
				Text:      "⚠️ 硬盘 S.M.A.R.T 严重告警",
				Font:      Font{Family: "微软雅黑", PointSize: 14, Bold: true},
				TextColor: walk.RGB(0xFF, 0xFF, 0xFF),
			},
			Label{
				Text:      body,
				Font:      Font{Family: "微软雅黑", PointSize: 11},
				TextColor: walk.RGB(0xFF, 0xF0, 0xF0),
			},
			PushButton{
				Text:      "关闭告警",
				OnClicked: func() { mw.Close() },
			},
		},
	}).Create()
	if err != nil {
		walk.MsgBox(nil, "⚠️ 硬盘告警", body, walk.MsgBoxIconWarning)
		return
	}

	// 红色背景 + 置顶
	red, _ := walk.NewSolidColorBrush(walk.RGB(0xC8, 0x1E, 0x1E))
	mw.SetBackground(red)
	setWindowTopMost(mw)

	go func() {
		mw.Show()
		mw.Run()
	}()
}

// setWindowTopMost 通过 Win32 SetWindowPos 把 walk 窗口置顶。
func setWindowTopMost(f *walk.MainWindow) {
	if f != nil {
		const SWP_NOSIZE = 0x0001
		const SWP_NOMOVE = 0x0002
		const SWP_SHOWWINDOW = 0x0040
		const HWND_TOPMOST = ^uintptr(0) // -1
		windows.NewLazySystemDLL("user32.dll").NewProc("SetWindowPos").Call(
			uintptr(f.Handle()),
			HWND_TOPMOST,
			0, 0, 0, 0,
			SWP_NOMOVE|SWP_NOSIZE|SWP_SHOWWINDOW,
		)
	}
}
