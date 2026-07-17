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
					{Title: "Raw值", Width: 150},
					{Title: "当前值", Width: 80},
					{Title: "最差", Width: 60},
					{Title: "阈值", Width: 60},
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
	raw      string
	current  string
	worst    string
	limit    string
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
		dname := fmt.Sprintf("PhysicalDrive%d  %s", d.Index, diskSummary(d))
		for _, a := range d.Attrs {
			k := [2]int{d.Index, a.ID}
			sev := vmap[k]
			r := reportRow{
				disk:     dname,
				kind:     string(d.Kind),
				id:       a.ID,
				name:     a.Name,
				raw:      attrRawStr(a),
				current:  attrCurrentStr(a),
				worst:    attrWorstStr(a),
				limit:    threshStr(a.Thresh),
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
		return r.raw
	case 5:
		return r.current
	case 6:
		return r.worst
	case 7:
		return r.limit
	case 8:
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

func attrRawStr(a smart.Attr) string {
	if a.Kind == "nvme" {
		switch a.ID {
		case smart.NVMeTemperature:
			return fmt.Sprintf("%d°C", int(a.Raw)-273)
		case smart.NVMeAvailableSpare, smart.NVMeAvailSpareThresh, smart.NVMePercentUsed:
			return fmt.Sprintf("%d%%", a.Raw)
		case smart.NVMeDataUnitsRead, smart.NVMeDataUnitsWritten:
			return fmt.Sprintf("%.2f TB (%d units)", float64(a.Raw)*512000.0/1e12, a.Raw)
		case smart.NVMePowerOnHours:
			return fmt.Sprintf("%d h", a.Raw)
		case smart.NVMeWarningTempTime, smart.NVMeCriticalTempTime:
			return fmt.Sprintf("%d min", a.Raw)
		}
	}
	if a.ID == 0xC2 || a.ID == 0xB9 || a.ID == 0xBE {
		current := a.Raw & 0xFF
		minimum := (a.Raw >> 8) & 0xFF
		maximum := (a.Raw >> 16) & 0xFF
		if minimum != 0 || maximum != 0 {
			return fmt.Sprintf("%d°C (min %d°C, max %d°C)", current, minimum, maximum)
		}
		return fmt.Sprintf("%d°C (raw %d)", current, a.Raw)
	}
	if a.ID == 0x09 {
		return fmt.Sprintf("%d h", a.Raw)
	}
	if a.ID == 0x0C {
		return fmt.Sprintf("%d", a.Raw)
	}
	return fmt.Sprintf("%d", a.Raw)
}

func diskSummary(d smart.Disk) string {
	summary := d.Model
	if d.SizeGB > 0 {
		summary = fmt.Sprintf("%s (%.0f GB)", summary, d.SizeGB)
	}
	if len(d.Attrs) == 0 {
		return summary + " [SMART未读取]"
	}
	if d.SmartStatusKnown {
		if d.SmartStatusPassed {
			return summary + " [SMART通过]"
		}
		return summary + " [SMART失败]"
	}
	return summary + " [SMART数据已读取]"
}

func smartStatusText(d smart.Disk) string {
	if !d.SmartStatusKnown {
		return "未知"
	}
	if d.SmartStatusPassed {
		return "通过"
	}
	return "失败"
}

func attrCurrentStr(a smart.Attr) string {
	if a.Kind == "nvme" {
		return "-"
	}
	return fmt.Sprintf("%d", a.Value)
}

func attrWorstStr(a smart.Attr) string {
	if a.Kind == "nvme" {
		return "-"
	}
	return fmt.Sprintf("%d", a.Worst)
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
