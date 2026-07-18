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
	banner     *walk.Label
}

// RunReport 显示报表。异常统一显示在主窗口横幅和表格中，不创建第二个告警窗口。
func RunReport(disks []smart.Disk, violations []health.Violation) error {
	return RunReportWithStatus(disks, violations, "")
}

// RunReportWithStatus displays an initial operational status in the main
// window. It is used for startup failures so the application never needs a
// blocking alert dialog to tell the user what happened.
func RunReportWithStatus(disks []smart.Disk, violations []health.Violation, status string) error {
	rw := &ReportWin{disks: disks, violations: violations}
	model := &reportModel{disks: disks, violations: violations}
	bannerText, bannerColor := reportBanner(disks, violations, status)

	err := MainWindow{
		AssignTo: &rw.MainWindow,
		Title:    "S.M.A.R.T 健康检查报告",
		MinSize:  Size{Width: 900, Height: 560},
		Size:     Size{Width: 1000, Height: 640},
		Layout:   VBox{Margins: Margins{Left: 8, Top: 8, Right: 8, Bottom: 8}},
		Children: []Widget{
			Label{
				AssignTo:  &rw.banner,
				Text:      bannerText,
				Font:      Font{Family: "微软雅黑", PointSize: 11},
				TextColor: bannerColor,
			},
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
					{Title: "标志", Width: 70},
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
						Text:      "复制异常条目",
						MinSize:   Size{Width: 140, Height: 30},
						OnClicked: func() { rw.copyReport() },
					},
					PushButton{
						Text:      "模拟异常验证",
						MinSize:   Size{Width: 120, Height: 30},
						OnClicked: func() { rw.simulateFailures() },
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

	rw.Run()
	return nil
}

func reportBanner(disks []smart.Disk, violations []health.Violation, status string) (string, walk.Color) {
	text, color := alertBannerText(disks, violations), alertBannerColor(disks, violations)
	if status != "" {
		return status + "\n" + text, walk.RGB(0xB0, 0x20, 0x20)
	}
	return text, color
}

// ===== 表格模型 =====

type reportModel struct {
	disks        []smart.Disk
	violations   []health.Violation
	rows         []reportRow
	built        bool
	rowsReset    walk.Event
	rowChanged   walk.IntEvent
	rowsChanged  walk.IntRangeEvent
	rowsInserted walk.IntRangeEvent
	rowsRemoved  walk.IntRangeEvent
}

type reportRow struct {
	disk     string
	kind     string
	id       int
	flags    string
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
		for _, v := range m.violations {
			if v.DiskIndex != d.Index || v.AttrID > 0 {
				continue
			}
			m.rows = append(m.rows, reportRow{
				disk:     dname,
				kind:     string(d.Kind),
				id:       v.AttrID,
				flags:    "-",
				name:     v.AttrName,
				raw:      "-",
				current:  v.Current,
				worst:    "-",
				limit:    v.Limit,
				status:   statusForSeverity(v.Severity),
				severity: v.Severity,
			})
		}
		if len(d.Attrs) == 0 {
			name, status := "SMART 数据未读取", "❔ 未读取"
			if d.Kind == smart.KindUnknown {
				name, status = "SMART 不适用", "— 不适用"
			}
			m.rows = append(m.rows, reportRow{
				disk:     dname,
				kind:     string(d.Kind),
				id:       -1,
				flags:    "-",
				name:     name,
				raw:      "-",
				current:  "-",
				worst:    "-",
				limit:    "-",
				status:   status,
				severity: "unknown",
			})
			continue
		}
		for _, a := range d.Attrs {
			k := [2]int{d.Index, a.ID}
			sev := vmap[k]
			r := reportRow{
				disk:     dname,
				kind:     string(d.Kind),
				id:       a.ID,
				flags:    attrFlagsStr(a),
				name:     a.Name,
				raw:      attrRawStrForModel(d.Model, a),
				current:  attrCurrentStr(a),
				worst:    attrWorstStr(a),
				limit:    threshStr(a.Thresh),
				severity: sev,
			}
			r.status = statusForSeverity(sev)
			m.rows = append(m.rows, r)
		}
	}
	m.built = true
}

func statusForSeverity(severity string) string {
	switch severity {
	case "critical":
		return "❌ 严重"
	case "warning":
		return "⚠️ 警告"
	default:
		return "✅"
	}
}

func (m *reportModel) RowCount() int {
	m.build()
	return len(m.rows)
}

// 事件必须在模型整个生命周期内保持同一实例。TableView 更换模型时会从
// 旧事件解绑；若每次返回新事件，Walk 会因解绑空处理器列表而发生 panic。
func (m *reportModel) RowsReset() *walk.Event            { return &m.rowsReset }
func (m *reportModel) RowChanged() *walk.IntEvent        { return &m.rowChanged }
func (m *reportModel) RowsChanged() *walk.IntRangeEvent  { return &m.rowsChanged }
func (m *reportModel) RowsInserted() *walk.IntRangeEvent { return &m.rowsInserted }
func (m *reportModel) RowsRemoved() *walk.IntRangeEvent  { return &m.rowsRemoved }

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
		if r.id < 0 {
			return "-"
		}
		return fmt.Sprintf("0x%02X", r.id)
	case 3:
		return r.flags
	case 4:
		return r.name
	case 5:
		return r.raw
	case 6:
		return r.current
	case 7:
		return r.worst
	case 8:
		return r.limit
	case 9:
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
	case "unknown":
		c.BackgroundColor = walk.RGB(0xE8, 0xE8, 0xE8)
		c.TextColor = walk.RGB(0x55, 0x55, 0x55)
	default:
		c.BackgroundColor = walk.RGB(0xEE, 0xF7, 0xEE)
		c.TextColor = walk.RGB(0x22, 0x66, 0x22)
	}
}

func attrRawStr(a smart.Attr) string {
	if a.Kind == "nvme" {
		switch a.ID {
		case smart.NVMeCriticalWarning, smart.NVMeEnduranceGroupCriticalWarning:
			return fmt.Sprintf("0x%02X", a.Raw)
		case smart.NVMeTemperature:
			if a.Raw == 0 {
				return "N/A"
			}
			return fmt.Sprintf("%d°C", int(a.Raw)-273)
		case smart.NVMeAvailableSpare, smart.NVMeAvailSpareThresh, smart.NVMePercentUsed:
			return fmt.Sprintf("%d%%", a.Raw)
		case smart.NVMeDataUnitsRead, smart.NVMeDataUnitsWritten:
			units := float64(a.Raw) + float64(a.RawHigh)*18446744073709551616.0
			if a.RawHigh == 0 {
				return fmt.Sprintf("%.2f TB (%d units)", units*512000.0/1e12, a.Raw)
			}
			return fmt.Sprintf("%.2f TB (0x%016X%016X units)", units*512000.0/1e12, a.RawHigh, a.Raw)
		case smart.NVMePowerOnHours:
			return nvmeCounterStr(a) + " h"
		case smart.NVMeWarningTempTime, smart.NVMeCriticalTempTime:
			return fmt.Sprintf("%d min", a.Raw)
		case smart.NVMeMediaErrors, smart.NVMePowerCycles, smart.NVMeUnsafeShutdowns, smart.NVMeErrorInfoEntries:
			return nvmeCounterStr(a)
		}
		if a.ID >= smart.NVMeTemperatureSensor1 && a.ID <= smart.NVMeTemperatureSensor8 {
			if a.Raw == 0 {
				return "N/A"
			}
			return fmt.Sprintf("%d°C", int(a.Raw)-273)
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

// attrRawStrForModel applies only the vendor/unit mappings verified against
// CrystalDiskInfo reports. Unknown models retain the generic raw display.
func attrRawStrForModel(model string, a smart.Attr) string {
	if a.Kind == "ata" {
		if smart.ATATemperatureAttributeForModel(model, a.ID) {
			current := a.Raw & 0xFF
			minimum := (a.Raw >> 8) & 0xFF
			maximum := (a.Raw >> 16) & 0xFF
			if minimum != 0 || maximum != 0 {
				return fmt.Sprintf("%d°C (min %d°C, max %d°C)", current, minimum, maximum)
			}
			return fmt.Sprintf("%d°C (raw %d)", current, a.Raw)
		}
		if smart.IsYMTCSATAModel(model) && (a.ID == 0xF1 || a.ID == 0xF2) {
			// CrystalDiskInfo's IsSsdYmtc selects 512-byte host I/O units.
			// Convert sectors to GiB, retaining the raw unit for traceability.
			return fmt.Sprintf("%.2f GiB (%d × 512 B)", float64(a.Raw)/(2*1024*1024), a.Raw)
		}
		m := strings.ToLower(model)
		switch {
		case strings.Contains(m, "kioxia") && a.ID == 0xF1:
			// CrystalDiskInfo's IsSsdKioxia selects 32 MB host I/O units;
			// its F1 handling converts the raw counter to GB by dividing by 32.
			return fmt.Sprintf("%d GB (%d × 32 MB)", a.Raw/32, a.Raw)
		case strings.Contains(m, "wd blue") && (a.ID == 0xF1 || a.ID == 0xF2):
			// WD Blue SA510 exposes these counters directly in GB.
			return fmt.Sprintf("%d GB", a.Raw)
		}
	}
	return attrRawStr(a)
}

func nvmeCounterStr(a smart.Attr) string {
	if a.RawHigh == 0 {
		return fmt.Sprintf("%d", a.Raw)
	}
	return fmt.Sprintf("0x%016X%016X", a.RawHigh, a.Raw)
}

func diskSummary(d smart.Disk) string {
	summary := d.Model
	if d.SizeGB > 0 {
		summary = fmt.Sprintf("%s (%.0f GB)", summary, d.SizeGB)
	}
	if life, ok := smart.ATAHealthPercentForModel(d.Model, d.Attrs); ok {
		summary = fmt.Sprintf("%s (健康度 %d%%)", summary, life)
	}
	if len(d.Attrs) == 0 {
		if d.Kind == smart.KindUnknown {
			return summary + " [SMART不适用]"
		}
		return summary + " [SMART未读取]"
	}
	if d.SmartStatusKnown {
		if d.SmartStatusPassed {
			if hasInvalidATASMARTChecksum(d) {
				return summary + " [SMART通过/校验和异常]"
			}
			return summary + " [SMART通过]"
		}
		return summary + " [SMART失败]"
	}
	if hasInvalidATASMARTChecksum(d) {
		return summary + " [SMART校验和异常]"
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

func smartChecksumText(d smart.Disk) string {
	if !d.SMARTChecksumKnown {
		return "未知"
	}
	if d.SMARTChecksumValid {
		return "有效"
	}
	return "异常"
}

func smartThresholdChecksumText(d smart.Disk) string {
	if !d.SMARTThresholdChecksumKnown {
		return "未知"
	}
	if d.SMARTThresholdChecksumValid {
		return "有效"
	}
	return "异常"
}

func hasInvalidATASMARTChecksum(d smart.Disk) bool {
	return (d.SMARTChecksumKnown && !d.SMARTChecksumValid) ||
		(d.SMARTThresholdChecksumKnown && !d.SMARTThresholdChecksumValid)
}

func attrCurrentStr(a smart.Attr) string {
	if a.Kind == "nvme" {
		return "-"
	}
	return fmt.Sprintf("%d", a.Value)
}

func attrFlagsStr(a smart.Attr) string {
	if a.Kind == "nvme" {
		return "-"
	}
	return fmt.Sprintf("0x%04X", a.Flags)
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

func unreadSMARTCount(disks []smart.Disk) int {
	count := 0
	for _, d := range disks {
		if isSMARTApplicable(d) && len(d.Attrs) == 0 {
			count++
		}
	}
	return count
}

func smartApplicableCount(disks []smart.Disk) int {
	count := 0
	for _, d := range disks {
		if isSMARTApplicable(d) {
			count++
		}
	}
	return count
}

func isSMARTApplicable(d smart.Disk) bool {
	return d.Kind != smart.KindUnknown
}

func alertBannerText(disks []smart.Disk, vs []health.Violation) string {
	if len(vs) == 0 {
		if unread := unreadSMARTCount(disks); unread > 0 {
			return fmt.Sprintf("⚠️ %d 块磁盘未读取到 SMART 数据，无法得出完整健康结论", unread)
		}
		if smartApplicableCount(disks) == 0 {
			return "ℹ️ 未发现支持 SMART 的物理磁盘"
		}
		return "✅ 所有监测的 SMART 指标均处于安全范围"
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
	if unread := unreadSMARTCount(disks); unread > 0 {
		lines = append(lines, fmt.Sprintf("  ⚠️ %d 块磁盘未读取到 SMART 数据", unread))
	}
	return strings.Join(lines, "\n")
}

func alertBannerColor(disks []smart.Disk, vs []health.Violation) walk.Color {
	if len(vs) > 0 {
		return walk.RGB(0x7A, 0x00, 0x00)
	}
	if unreadSMARTCount(disks) > 0 {
		return walk.RGB(0xB0, 0x60, 0x00)
	}
	if smartApplicableCount(disks) == 0 {
		return walk.RGB(0x44, 0x44, 0x44)
	}
	return walk.RGB(0x1B, 0x7A, 0x2F)
}

// ===== 行为 =====

func (rw *ReportWin) copyReport() {
	rpt := buildExceptionReport(rw.disks, rw.violations)
	if err := WriteText(rpt); err != nil {
		rw.showStatus("复制失败："+err.Error(), walk.RGB(0xB0, 0x20, 0x20))
		return
	}
	message := "异常条目已复制到剪贴板，可直接粘贴到反馈表格。"
	if len(rw.violations) == 0 {
		message = "当前未发现异常，已复制状态提示。"
	}
	rw.showStatus(message, walk.RGB(0x1B, 0x7A, 0x2F))
}

func (rw *ReportWin) rescan() {
	disks, err := smart.Discover()
	if err != nil {
		rw.showStatus("扫描失败："+err.Error(), walk.RGB(0xB0, 0x20, 0x20))
		return
	}
	if err := rw.setReportData(disks); err != nil {
		rw.showStatus("刷新失败："+err.Error(), walk.RGB(0xB0, 0x20, 0x20))
	}
}

func (rw *ReportWin) simulateFailures() {
	if err := rw.setReportData(SimulatedFailureDisks()); err != nil {
		rw.showStatus("模拟失败："+err.Error(), walk.RGB(0xB0, 0x20, 0x20))
		return
	}
	rw.showStatus("🧪 模拟异常验证（仅内存数据；点击“重新扫描”恢复真实磁盘）", walk.RGB(0xB0, 0x60, 0x00))
}

// showStatus 将操作结果直接写入主窗口横幅，避免模态对话框阻塞界面。
func (rw *ReportWin) showStatus(message string, color walk.Color) {
	if rw.banner == nil {
		return
	}
	_ = rw.banner.SetText(message + "\n" + alertBannerText(rw.disks, rw.violations))
	rw.banner.SetTextColor(color)
}

func (rw *ReportWin) setReportData(disks []smart.Disk) error {
	rw.disks = disks
	rw.violations = health.Evaluate(disks)
	if rw.banner != nil {
		if err := rw.banner.SetText(alertBannerText(disks, rw.violations)); err != nil {
			return err
		}
		rw.banner.SetTextColor(alertBannerColor(disks, rw.violations))
	}
	if rw.tv != nil {
		if err := rw.tv.SetModel(&reportModel{disks: disks, violations: rw.violations}); err != nil {
			return err
		}
	}
	return nil
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
