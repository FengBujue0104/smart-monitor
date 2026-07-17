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

// buildTextReport 生成适合粘贴的纯文本报表（UTF-16 LE 由 clipboard 层处理）。
func buildTextReport(disks []smart.Disk, vs []health.Violation) string {
	var b strings.Builder
	b.WriteString("====== S.M.A.R.T 检测报告 ======\n")
	b.WriteString(fmt.Sprintf("时间: %s\n", time.Now().Format("2006-01-02 15:04:05")))
	b.WriteString(fmt.Sprintf("主机: %s\n", hostName()))
	b.WriteString(fmt.Sprintf("磁盘数: %d\n\n", len(disks)))

	for _, d := range disks {
		b.WriteString(fmt.Sprintf("[磁盘 %d] %s  (%s)  容量:%.1f GB  SMART:%s  校验和:%s  S/N:%s  FW:%s\n",
			d.Index, d.Model, d.Kind, d.SizeGB, smartStatusText(d), smartChecksumText(d), d.Serial, d.Firmware))
		if len(d.Attrs) == 0 {
			b.WriteString("  (未读取到 SMART 属性)\n\n")
			continue
		}
		for _, a := range d.Attrs {
			mark := "✅"
			for _, v := range vs {
				if v.DiskIndex == d.Index && v.AttrID == a.ID {
					if v.Severity == "critical" {
						mark = "❌"
					} else {
						mark = "⚠️"
					}
					break
				}
			}
			b.WriteString(fmt.Sprintf("  %s 0x%02X %-30s flags=%s raw=%s  val=%s worst=%s thresh=%s\n",
				mark, a.ID, a.Name, attrFlagsStr(a), attrRawStr(a), attrCurrentStr(a), attrWorstStr(a), threshStr(a.Thresh)))
		}
		b.WriteString("\n")
	}

	if len(vs) > 0 {
		b.WriteString("====== 告警汇总 ======\n")
		for _, v := range vs {
			mark := "⚠️ 警告"
			if v.Severity == "critical" {
				mark = "❌ 严重"
			}
			b.WriteString(fmt.Sprintf("  %s [Disk%d] %s: %s (阈值 %s)\n",
				mark, v.DiskIndex, v.AttrName, v.Current, v.Limit))
		}
	} else {
		if unread := unreadSMARTCount(disks); unread > 0 {
			b.WriteString(fmt.Sprintf("结论: %d 块磁盘未读取到 SMART 数据，无法得出完整健康结论。\n", unread))
		} else {
			b.WriteString("结论: 所有监测指标在安全范围内。\n")
		}
	}
	b.WriteString("============================\n")
	return b.String()
}
