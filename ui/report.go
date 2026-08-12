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
	stampLbl   *walk.Label
	copyBtn    *walk.PushButton
	simBtn     *walk.PushButton
	rescanBtn  *walk.PushButton
	rescanning bool
	closed     bool     // 窗口已关闭：扫描完成回调不得再触碰已销毁的控件
	lastTitle  string   // 上次设置的标题，用于“新出现严重告警”时蜂鸣一次
}

type discoveryResult struct {
	disks []smart.Disk
	err   error
}

func discoverAsync(discover func() ([]smart.Disk, error)) <-chan discoveryResult {
	result := make(chan discoveryResult, 1)
	go func() {
		// 扫描 goroutine 内的 panic 必须在发送结果前转成 error，否则调用方
		// 会永久阻塞在 <-result（windowsgui 构建无控制台，表现为“点重新扫描
		// 程序没反应”）。WMI 底层（go-ole）偶发 panic，这里兜底。
		defer func() {
			if r := recover(); r != nil {
				result <- discoveryResult{disks: nil, err: fmt.Errorf("扫描过程中发生异常: %v", r)}
			}
		}()
		disks, err := discover()
		result <- discoveryResult{disks: disks, err: err}
	}()
	return result
}

// RunReportWithStatus displays an initial operational status in the main
// window. It is used for startup failures so the application never needs a
// blocking alert dialog to tell the user what happened.
func RunReportWithStatus(disks []smart.Disk, violations []health.Violation, status string) error {
	rw := &ReportWin{disks: disks, violations: violations}
	model := &reportModel{disks: disks, violations: violations}
	if _, err := createReportWindow(rw, model, status); err != nil {
		return err
	}
	rw.Run()
	return nil
}

// RunReportWithScan shows the window immediately with a "scanning" banner and
// runs the disk scan in the background, so the UI never sits frozen for the
// few seconds a USB/RAID probe can take. The scan result fills the report via
// the same rescan path (first scan is just the initial rescan).
func RunReportWithScan(discover func() ([]smart.Disk, error)) error {
	rw := &ReportWin{}
	model := &reportModel{}
	if _, err := createReportWindow(rw, model, "正在扫描磁盘，请稍候…"); err != nil {
		return err
	}
	rw.rescanWith(discover)
	rw.Run()
	return nil
}

// createReportWindow builds the main report window with the given banner
// status and returns the populated ReportWin. Shared by the synchronous
// report and the asynchronous scan entry points.
func createReportWindow(rw *ReportWin, model *reportModel, status string) (*ReportWin, error) {
	bannerText, bannerColor := reportBanner(rw.disks, rw.violations, status)

	err := MainWindow{
		AssignTo: &rw.MainWindow,
		Title:    "S.M.A.R.T 健康检查报告",
		// 11 列总宽约 1270px，默认窗口放大到能一屏容纳，避免用户横向滚动。
		MinSize: Size{Width: 900, Height: 560},
		Size:    Size{Width: 1320, Height: 700},
		Layout:  VBox{Margins: Margins{Left: 8, Top: 8, Right: 8, Bottom: 8}},
		Children: []Widget{
			Label{
				AssignTo:  &rw.banner,
				Text:      bannerText,
				Font:      Font{Family: "微软雅黑", PointSize: 11},
				TextColor: bannerColor,
			},
			Label{
				AssignTo:  &rw.stampLbl,
				Text:      rw.stampText(),
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
					{Title: "属性名", Width: 200},
					{Title: "含义", Width: 260},
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
						AssignTo:  &rw.copyBtn,
						Text:      "复制异常条目",
						MinSize:   Size{Width: 140, Height: 30},
						OnClicked: func() { rw.copyReport() },
					},
					PushButton{
						AssignTo:  &rw.simBtn,
						Text:      "模拟异常验证",
						MinSize:   Size{Width: 120, Height: 30},
						OnClicked: func() { rw.simulateFailures() },
					},
					PushButton{
						AssignTo:  &rw.rescanBtn,
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
		return nil, err
	}
	// 记录窗口关闭：扫描完成回调（通过 Synchronize 在 UI 线程执行）若在
	// 窗口销毁后触碰控件会导致崩溃，必须放弃刷新。
	rw.Closing().Attach(func(_ *bool, _ walk.CloseReason) {
		rw.closed = true
	})
	return rw, nil
}

func reportBanner(disks []smart.Disk, violations []health.Violation, status string) (string, walk.Color) {
	if status != "" {
		// 初始/错误提示：只显示状态文本，不附加“未发现磁盘”等会误导的噪音行。
		return status, walk.RGB(0xB0, 0x60, 0x00)
	}
	return alertBannerText(disks, violations), alertBannerColor(disks, violations)
}

// stampText 生成“检测时间”行。每次扫描（含重新扫描）都应刷新，
// 否则挂机后看到的是旧的检测时间，误判数据新鲜度。
func (rw *ReportWin) stampText() string {
	return fmt.Sprintf("检测时间: %s | 主机: %s   |  注：厂商只为少数关键属性设置阈值（与 CrystalDiskInfo 一致），其余条目按 Raw/Value 动态判断",
		time.Now().Format("2006-01-02 15:04:05"), hostName())
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
	meaning  string
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
			current := "-"
			if d.SMARTReadError != "" {
				current = d.SMARTReadError
			}
			m.rows = append(m.rows, reportRow{
				disk:     dname,
				kind:     string(d.Kind),
				id:       -1,
				flags:    "-",
				name:     name,
				raw:      "-",
				current:  current,
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
				meaning:  smart.AttrMeaning(d.Model, a),
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
		return r.meaning
	case 6:
		return r.raw
	case 7:
		return r.current
	case 8:
		return r.worst
	case 9:
		return r.limit
	case 10:
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
		case smart.NVMeTemperature, smart.NVMeWarningCompositeTempThreshold, smart.NVMeCriticalCompositeTempThreshold:
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
		if unit, ok := smart.ATACounterUnitForModel(model, a.ID); ok {
			switch unit {
			case smart.ATACounterUnit512B:
				return fmt.Sprintf("%.2f GiB (%d × 512 B)", float64(a.Raw)/(2*1024*1024), a.Raw)
			case smart.ATACounterUnit1MB:
				return fmt.Sprintf("%d GB (%d × 1 MB)", a.Raw/1024, a.Raw)
			case smart.ATACounterUnit32MB:
				return fmt.Sprintf("%d GB (%d × 32 MB)", a.Raw/32, a.Raw)
			case smart.ATACounterUnitGB:
				return fmt.Sprintf("%d GB", a.Raw)
			}
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
	if d.SMARTTransport != "" {
		summary += " [" + d.SMARTTransport + "]"
	}
	if profile := smart.ATAVendorProfileName(d.Model); profile != "" {
		summary += " [" + profile + "]"
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

// banner 折叠：违规/未读明细可能非常多（几十块盘×多条违规），全列出会把
// 表格挤出窗口。超出上限的行折叠为一句提示，完整列表在表格与复制报表中。
const (
	maxBannerViolationLines = 6
	maxBannerUnreadLines    = 3
)

func unreadSMARTDetails(disks []smart.Disk) []string {
	var details []string
	for _, d := range disks {
		if !isSMARTApplicable(d) || len(d.Attrs) != 0 || d.SMARTReadError == "" {
			continue
		}
		reason := []rune(d.SMARTReadError)
		if len(reason) > 120 {
			reason = append(reason[:119], '…')
		}
		details = append(details, fmt.Sprintf("  Disk%d: %s", d.Index, string(reason)))
	}
	if len(details) > maxBannerUnreadLines {
		extra := len(details) - maxBannerUnreadLines
		details = details[:maxBannerUnreadLines]
		details = append(details, fmt.Sprintf("  …及另外 %d 块磁盘，详见下方表格", extra))
	}
	return details
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
			lines := []string{fmt.Sprintf("⚠️ %d 块磁盘未读取到 SMART 数据，无法得出完整健康结论", unread)}
			return strings.Join(append(lines, unreadSMARTDetails(disks)...), "\n")
		}
		if smartApplicableCount(disks) == 0 {
			return "ℹ️ 未发现支持 SMART 的物理磁盘"
		}
		return "✅ 所有监测的 SMART 指标均处于安全范围"
	}
	var lines []string
	shown, extra := 0, 0
	for _, v := range vs {
		if shown >= maxBannerViolationLines {
			extra++
			continue
		}
		mark := "⚠️"
		if v.Severity == "critical" {
			mark = "❌"
		}
		lines = append(lines, fmt.Sprintf("  %s [Disk%d] %s: %s (阈值 %s)",
			mark, v.DiskIndex, v.AttrName, v.Current, v.Limit))
		shown++
	}
	if extra > 0 {
		lines = append(lines, fmt.Sprintf("  …及另外 %d 项异常，详见下方表格", extra))
	}
	if unread := unreadSMARTCount(disks); unread > 0 {
		lines = append(lines, fmt.Sprintf("  ⚠️ %d 块磁盘未读取到 SMART 数据", unread))
		lines = append(lines, unreadSMARTDetails(disks)...)
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
	// 扫描未完成时 disks 仍为空，复制会得到“未检测到异常”的假结论；
	// 按钮已在扫描期间禁用，这里再加一层防御。
	if rw.disks == nil {
		rw.showStatus("扫描尚未完成，请稍候再复制。", walk.RGB(0xB0, 0x60, 0x00))
		return
	}
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
	rw.rescanWith(smart.DiscoverWithFallback)
}

// rescanWith runs a disk scan in the background and refreshes the report.
// It is shared by the "重新扫描" button and the initial startup scan, so a
// duplicate invocation is rejected by the rescanning flag in both cases.
func (rw *ReportWin) rescanWith(discover func() ([]smart.Disk, error)) {
	if rw.rescanning {
		return
	}
	rw.rescanning = true
	rw.setActionsEnabled(false)
	rw.showStatus("正在扫描磁盘，请稍候…", walk.RGB(0xB0, 0x60, 0x00))

	// SMART IOCTL and WMI probing can take noticeable time on USB/RAID
	// controllers. Keep that work outside Walk's UI thread, then marshal only
	// the model replacement back to the window.
	result := discoverAsync(discover)
	go func() {
		outcome := <-result
		// 兜底：discoverAsync 已把扫描 goroutine 的 panic 转成 error；
		// 这里的 recover 只保护本 goroutine 后续的 UI 更新代码。
		defer func() {
			if r := recover(); r != nil {
				rw.Synchronize(func() {
					if rw.closed {
						return
					}
					rw.rescanning = false
					rw.setActionsEnabled(true)
					rw.showStatus("扫描异常："+fmt.Sprintf("%v", r), walk.RGB(0xB0, 0x20, 0x20))
				})
			}
		}()
		rw.Synchronize(func() {
			if rw.closed {
				return
			}
			rw.rescanning = false
			rw.setActionsEnabled(true)
			switch {
			case outcome.err != nil:
				rw.showStatus("扫描失败："+outcome.err.Error(), walk.RGB(0xB0, 0x20, 0x20))
			case len(outcome.disks) == 0:
				rw.showStatus("⚠ 未找到任何物理磁盘。", walk.RGB(0xB0, 0x60, 0x00))
			default:
				if err := rw.setReportData(outcome.disks); err != nil {
					rw.showStatus("刷新失败："+err.Error(), walk.RGB(0xB0, 0x20, 0x20))
				}
			}
		})
	}()
}

func (rw *ReportWin) simulateFailures() {
	if err := rw.setReportData(SimulatedFailureDisks()); err != nil {
		rw.showStatus("模拟失败："+err.Error(), walk.RGB(0xB0, 0x20, 0x20))
		return
	}
	rw.showStatus("🧪 模拟异常验证（仅内存数据；点击“重新扫描”恢复真实磁盘）", walk.RGB(0xB0, 0x60, 0x00))
}

// showStatus 将操作结果直接写入主窗口横幅，避免模态对话框阻塞界面。
// 首次扫描完成前 rw.disks 为空，此时不拼接告警行（否则会显示误导性的
// “未发现支持 SMART 的物理磁盘”）。
func (rw *ReportWin) showStatus(message string, color walk.Color) {
	if rw.banner == nil {
		return
	}
	text := message
	if rw.disks != nil {
		text += "\n" + alertBannerText(rw.disks, rw.violations)
	}
	_ = rw.banner.SetText(text)
	rw.banner.SetTextColor(color)
}

// setActionsEnabled 在扫描期间禁用所有操作按钮。扫描未完成时“复制异常条目”
// 会复制出“未检测到异常”的假结论，“重新扫描”/“模拟异常验证”也不该并发触发。
func (rw *ReportWin) setActionsEnabled(enabled bool) {
	for _, b := range []*walk.PushButton{rw.copyBtn, rw.simBtn, rw.rescanBtn} {
		if b != nil {
			b.SetEnabled(enabled)
		}
	}
}

// reportTitle 生成随健康状态变化的窗口标题，让任务栏也能看出是否有严重告警。
func reportTitle(vs []health.Violation) string {
	critical, warnings := 0, 0
	for _, v := range vs {
		if v.Severity == health.Critical {
			critical++
		} else {
			warnings++
		}
	}
	switch {
	case critical > 0:
		return fmt.Sprintf("❌ S.M.A.R.T 健康检查报告 — %d 项严重告警", critical)
	case warnings > 0:
		return fmt.Sprintf("⚠️ S.M.A.R.T 健康检查报告 — %d 项警告", warnings)
	default:
		return "✅ S.M.A.R.T 健康检查报告"
	}
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
	if rw.stampLbl != nil {
		if err := rw.stampLbl.SetText(rw.stampText()); err != nil {
			return err
		}
	}
	if rw.tv != nil {
		if err := rw.tv.SetModel(&reportModel{disks: disks, violations: rw.violations}); err != nil {
			return err
		}
	}
	// 标题随健康状态更新；状态从“无严重”变为“有严重”时蜂鸣一次提醒。
	title := reportTitle(rw.violations)
	if rw.MainWindow != nil {
		if err := rw.SetTitle(title); err != nil {
			return err
		}
	}
	if strings.Contains(title, "❌") && !strings.Contains(rw.lastTitle, "❌") {
		alertBeep()
	}
	rw.lastTitle = title
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
