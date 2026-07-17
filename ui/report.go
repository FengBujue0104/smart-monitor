package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
	"golang.org/x/sys/windows"
	"smonitor/health"
	"smonitor/smart"
)

// ReportWin 主报表窗口
type ReportWin struct {
	*walk.MainWindow
	disks      []smart.Disk
	violations []health.Violation
	tv         *walk.TableView
}

// RunReport 显示报表。violations 非空时弹出红色告警条。
func RunReport(disks []smart.Disk, violations []health.Violation) error {
	rw := &ReportWin{disks: disks, violations: violations}
	model := &reportModel{disks: disks, violations: violations}

	err := MainWindow{
		AssignTo: &rw.MainWindow,
		Title:    "S.M.A.R.T 健康检查报告",
		MinSize:  Size{Width: 900, Height: 560},
		Size:     Size{Width: 1000, Height: 640},
		Layout:   VBox{Margins: Margins{Left: 8, Top: 8, Right: 8, Bottom: 8}},
		Children: []Widget{
			alertBanner(violations),
			Label{
				Text:      fmt.Sprintf("检测时间: %s | 主机: %s", time.Now().Format("2006-01-02 15:04:05"), hostName()),
				Font:      Font{Family: "微软雅黑", PointSize: 10},
				TextColor: walk.RGB(0x44, 0x44, 0x44),
			},
			TableView{
				AssignTo:            &rw.tv,
				AlternatingRowBG:    true,
				ColumnsOrderable:    true,
				MultiSelection:      false,
				Model:               model,
				MinSize:             Size{Width: 400, Height: 400},
				LastColumnStretched: true,
				Columns: []TableViewColumn{
					{Title: "磁盘", Width: 200},
					{Title: "类型", Width: 50},
					{Title: "属性ID", Width: 60},
					{Title: "属性名", Width: 220},
					{Title: "当前值", Width: 90},
					{Title: "阈值", Width: 60},
					{Title: "归一化", Width: 70},
					{Title: "状态", Width: 80},
				},
			},
			Composite{
				Layout: HBox{MarginsZero: true},
				Children: []Widget{
					PushButton{
						Text:      "一键复制报表",
						MinSize:   Size{Width: 140, Height: 30},
						OnClicked: func() { rw.copyReport() },
					},
					PushButton{
						Text:      "重新扫描",
						MinSize:   Size{Width: 100, Height: 30},
						OnClicked: func() { rw.rescan() },
					},
					PushButton{
						Text:      "退出",
						MinSize:   Size{Width: 80, Height: 30},
						OnClicked: func() { rw.Close() },
					},
					HSpacer{},
				},
			},
		},
	}.Create()

	if err != nil {
		return err
	}

	if len(violations) > 0 {
		go showRedAlert(violations)
	}
	rw.Run()
	return nil
}

// ===== 表格模型 =====

type reportModel struct {
	disks      []smart.Disk
	violations []health.Violation
	rows       []reportRow
	built      bool
}

type reportRow struct {
	disk     string
	kind     string
	id       int
	name     string
	cur      string
	limit    string
	value    int
	worst    int
	status   string
	severity string
}

func (m *reportModel) build() {
	if m.built {
		return
	}
	vmap := map[[2]int]string{}
	for _, v := range m.violations {
		k := [2]int{v.DiskIndex, v.AttrID}
		if v.Severity == "critical" {
			vmap[k] = "critical"
		} else if vmap[k] != "critical" {
			vmap[k] = "warning"
		}
	}
	for _, d := range m.disks {
		dname := fmt.Sprintf("PhysicalDrive%d  %s", d.Index, d.Model)
		for _, a := range d.Attrs {
			k := [2]int{d.Index, a.ID}
			sev := vmap[k]
			r := reportRow{
				disk:     dname,
				kind:     string(d.Kind),
				id:       a.ID,
				name:     a.Name,
				cur:      attrCurStr(a),
				limit:    threshStr(a.Thresh),
				value:    a.Value,
				worst:    a.Worst,
				severity: sev,
			}
			switch sev {
			case "critical":
				r.status = "❌ 严重"
			case "warning":
				r.status = "⚠️ 警告"
			default:
				r.status = "✅"
			}
			m.rows = append(m.rows, r)
		}
	}
	m.built = true
}

func (m *reportModel) RowCount() int {
	m.build()
	return len(m.rows)
}

// 事件方法（静态数据，事件永不触发，返回空事件即可）。
func (m *reportModel) RowsReset() *walk.Event            { return &walk.Event{} }
func (m *reportModel) RowChanged() *walk.IntEvent        { return &walk.IntEvent{} }
func (m *reportModel) RowsChanged() *walk.IntRangeEvent  { return &walk.IntRangeEvent{} }
func (m *reportModel) RowsInserted() *walk.IntRangeEvent { return &walk.IntRangeEvent{} }
func (m *reportModel) RowsRemoved() *walk.IntRangeEvent  { return &walk.IntRangeEvent{} }

func (m *reportModel) Value(row, col int) interface{} {
	m.build()
	if row < 0 || row >= len(m.rows) {
		return ""
	}
	r := m.rows[row]
	switch col {
	case 0:
		return r.disk
	case 1:
		return r.kind
	case 2:
		return fmt.Sprintf("0x%02X", r.id)
	case 3:
		return r.name
	case 4:
		return r.cur
	case 5:
		return r.limit
	case 6:
		return fmt.Sprintf("C%d/W%d", r.value, r.worst)
	case 7:
		return r.status
	}
	return ""
}

func (m *reportModel) StyleCell(c *walk.CellStyle) {
	m.build()
	if c.Row() < 0 || c.Row() >= len(m.rows) {
		return
	}
	r := m.rows[c.Row()]
	switch r.severity {
	case "critical":
		c.BackgroundColor = walk.RGB(0xEE, 0x90, 0x90)
		c.TextColor = walk.RGB(0x99, 0x00, 0x00)
	case "warning":
		c.BackgroundColor = walk.RGB(0xEE, 0xD0, 0x80)
		c.TextColor = walk.RGB(0xB0, 0x60, 0x00)
	default:
		c.BackgroundColor = walk.RGB(0xEE, 0xF7, 0xEE)
		c.TextColor = walk.RGB(0x22, 0x66, 0x22)
	}
}

func attrCurStr(a smart.Attr) string {
	if a.Kind == "nvme" && a.ID == smart.NVMeTemperature {
		return fmt.Sprintf("%d°C", int(a.Raw)-273)
	}
	if a.ID == 0xC2 || a.ID == 0xB9 || a.ID == 0xBE {
		return fmt.Sprintf("%d°C", a.Raw&0xFF)
	}
	if a.Kind == "nvme" {
		switch a.ID {
		case smart.NVMeAvailableSpare, smart.NVMeAvailSpareThresh, smart.NVMePercentUsed:
			return fmt.Sprintf("%d%%", a.Raw)
		}
	}
	return fmt.Sprintf("%d", a.Raw)
}

func threshStr(t int) string {
	if t == 0 {
		return "-"
	}
	return fmt.Sprintf("%d", t)
}

// ===== 告警条 =====

func alertBanner(vs []health.Violation) Widget {
	if len(vs) == 0 {
		return Label{
			Text:      "✅ 所有监测的 SMART 指标均处于安全范围",
			Font:      Font{Family: "微软雅黑", PointSize: 11},
			TextColor: walk.RGB(0x1B, 0x7A, 0x2F),
		}
	}
	var lines []string
	for _, v := range vs {
		mark := "⚠️"
		if v.Severity == "critical" {
			mark = "❌"
		}
		lines = append(lines, fmt.Sprintf("  %s [Disk%d] %s: %s (阈值 %s)",
			mark, v.DiskIndex, v.AttrName, v.Current, v.Limit))
	}
	return TextEdit{
		ReadOnly:   true,
		Text:       strings.Join(lines, "\n"),
		TextColor:  walk.RGB(0x7A, 0x00, 0x00),
		Background: SolidColorBrush{Color: walk.RGB(0xFD, 0xDE, 0xDE)},
		Font:       Font{Family: "微软雅黑", PointSize: 11},
		MinSize:    Size{Width: 400, Height: 80},
	}
}

// ===== 行为 =====

func (rw *ReportWin) copyReport() {
	rpt := buildTextReport(rw.disks, rw.violations)
	if err := WriteText(rpt); err != nil {
		walk.MsgBox(rw, "复制失败", err.Error(), walk.MsgBoxIconError)
		return
	}
	walk.MsgBox(rw, "复制成功", "报表已复制到剪贴板。", walk.MsgBoxIconInformation)
}

func (rw *ReportWin) rescan() {
	disks, err := smart.Discover()
	if err != nil {
		walk.MsgBox(rw, "扫描失败", err.Error(), walk.MsgBoxIconError)
		return
	}
	rw.disks = disks
	rw.violations = health.Evaluate(disks)
	if rw.tv != nil {
		rw.tv.SetModel(&reportModel{disks: disks, violations: rw.violations})
	}
}

func hostName() string {
	const MAX = 256
	b := make([]uint16, MAX)
	n := uint32(MAX)
	if err := windows.GetComputerName(&b[0], &n); err != nil {
		return "unknown"
	}
	return strings.ToLower(windows.UTF16ToString(b[:n]))
}
