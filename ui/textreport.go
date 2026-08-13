package ui

import (
	"fmt"
	"strings"
	"time"

	"smonitor/health"
	"smonitor/smart"
)

// BuildTextReportForTest 是 buildTextReport 的导出版（供测试/CLI 用）。
func BuildTextReportForTest(disks []smart.Disk, vs []health.Violation) string {
	return buildTextReport(disks, vs)
}

// BuildExceptionReportForTest exposes the concise feedback-table report used
// by the copy button.
func BuildExceptionReportForTest(disks []smart.Disk, vs []health.Violation) string {
	return buildExceptionReport(disks, vs)
}

// buildExceptionReport returns a tab-separated exception list suitable for
// pasting directly into a feedback spreadsheet. It intentionally excludes all
// healthy SMART attributes and other verbose diagnostic data.
func buildExceptionReport(disks []smart.Disk, vs []health.Violation) string {
	if len(vs) == 0 {
		return "未检测到 S.M.A.R.T 异常。\n"
	}
	models := make(map[int]string, len(disks))
	for _, d := range disks {
		models[d.Index] = d.Model
	}
	var b strings.Builder
	b.WriteString("磁盘号\t型号\t异常项目\t当前值\t阈值\t级别\n")
	for _, v := range vs {
		model := v.DiskModel
		if model == "" {
			model = models[v.DiskIndex]
		}
		level := "警告"
		if v.Severity == health.Critical {
			level = "严重"
		}
		b.WriteString(fmt.Sprintf("Disk%d\t%s\t%s\t%s\t%s\t%s\n",
			v.DiskIndex, feedbackField(model), feedbackField(v.AttrName), feedbackField(v.Current), feedbackField(v.Limit), level))
	}
	return b.String()
}

func feedbackField(s string) string {
	s = strings.ReplaceAll(s, "\t", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	return strings.ReplaceAll(s, "\n", " ")
}

// buildTextReport 生成适合粘贴的纯文本报表（UTF-16 LE 由 clipboard 层处理）。
func buildTextReport(disks []smart.Disk, vs []health.Violation) string {
	var b strings.Builder
	b.WriteString("====== S.M.A.R.T 检测报告 ======\n")
	b.WriteString(fmt.Sprintf("时间: %s\n", time.Now().Format("2006-01-02 15:04:05")))
	b.WriteString(fmt.Sprintf("主机: %s\n", hostName()))
	b.WriteString(fmt.Sprintf("磁盘数: %d\n\n", len(disks)))

	for _, d := range disks {
		b.WriteString(fmt.Sprintf("[磁盘 %d] %s  (%s)  容量:%.1f GB  SMART:%s  属性校验和:%s  阈值校验和:%s  S/N:%s  FW:%s\n",
			d.Index, d.Model, d.Kind, d.SizeGB, smartStatusText(d), smartChecksumText(d), smartThresholdChecksumText(d), d.Serial, d.Firmware))
		if len(d.Attrs) == 0 {
			if d.Kind == smart.KindUnknown {
				b.WriteString("  (该总线类型不提供 SMART 数据)\n\n")
			} else {
				b.WriteString("  (未读取到 SMART 属性)")
				if d.SMARTReadError != "" {
					b.WriteString("  原因: " + d.SMARTReadError)
				}
				b.WriteString("\n\n")
			}
			continue
		}
		for _, a := range d.Attrs {
			mark := "      "
			for _, v := range vs {
				if v.DiskIndex == d.Index && v.AttrID == a.ID {
					if v.Severity == "critical" {
						mark = "[严重]"
					} else {
						mark = "[警告]"
					}
					break
				}
			}
			b.WriteString(fmt.Sprintf("  %s 0x%02X %-30s %-28s flags=%s raw=%s  val=%s worst=%s thresh=%s\n",
				mark, a.ID, a.Name, smart.AttrMeaning(d.Model, a), attrFlagsStr(a), attrRawStrForModel(d.Model, a), attrCurrentStr(a), attrWorstStr(a), threshStr(a.Thresh)))
		}
		b.WriteString("\n")
	}

	if len(vs) > 0 {
		b.WriteString("====== 告警汇总 ======\n")
		for _, v := range vs {
			mark := "[警告]"
			if v.Severity == "critical" {
				mark = "[严重]"
			}
			b.WriteString(fmt.Sprintf("  %s [Disk%d] %s: %s (阈值 %s)\n",
				mark, v.DiskIndex, v.AttrName, v.Current, v.Limit))
		}
	} else {
		if unread := unreadSMARTCount(disks); unread > 0 {
			b.WriteString(fmt.Sprintf("结论: %d 块磁盘未读取到 SMART 数据，无法得出完整健康结论。\n", unread))
		} else if smartApplicableCount(disks) == 0 {
			b.WriteString("结论: 未发现支持 SMART 的物理磁盘。\n")
		} else {
			b.WriteString("结论: 所有监测指标在安全范围内。\n")
		}
	}
	b.WriteString("============================\n")
	return b.String()
}
