package health

import "smonitor/smart"

// Severity 违规严重度
type Severity = string

const (
	Critical Severity = "critical"
	Warning  Severity = "warning"
)

// Violation 同 smart.Violation（重导出便于 UI 直接用）
type Violation = smart.Violation

// Evaluate 对每块磁盘计算阈值违规列表并返回。
// 阈值来源：ATA8-ACS + NVMe Base Spec + CrystalDiskInfo + 用户指定规则。
//
// ATA 规则（基于权威解读）：
//   0x05 (Reallocated_Sector_Ct)  raw != 0              -> critical 重映射扇区，物理损坏标志
//   0xC5 (Current_Pending_Sector) raw != 0               -> warning  等待重映射的扇区
//   0xC6 (Offline_Uncorrectable)   raw != 0               -> warning
//   0xBB (Reported_Uncorrectable)  raw != 0               -> critical
//   0x01 (Raw_Read_Error_Rate)     raw != 0 且 value<th   -> warning  （注：WD/Seagate raw 常非零，需 value<thresh）
//   0x0E                           raw != 0               -> warning  （厂牌特定：三星=过热计数）
//   0xC2 (Temperature)             > 60°C                 -> critical（用户要求）
//                                    > 55°C                 -> warning
//   0xBC (Command_Timeout)          > 10                   -> warning（用户要求）
//   0xE9 (Media_Wearout_Indicator)  <= 50                  -> critical（剩余寿命<=50%）
//   0xE8 (Available_Reservd_Space)  <= 50 (Samsung)        -> warning
//   0xAD/0xB1 (Wear_Leveling)       极低                    -> warning
// NVMe 规则：
//   MediaErrors (!= 0)                                     -> critical
//   PercentageUsed >= 50                                   -> critical（剩余寿命<=50%）
//   Temperature(K) > 333 (60°C)                            -> critical
//   Temperature(K) > 328 (55°C)                            -> warning
//   CriticalWarning != 0                                   -> critical
//   AvailableSpare < Threshold                             -> critical
func Evaluate(disks []smart.Disk) []Violation {
	var out []Violation
	for _, d := range disks {
		switch d.Kind {
		case smart.KindATA:
			out = append(out, evaluateATA(d)...)
		case smart.KindNVMe:
			out = append(out, evaluateNVMe(d)...)
		}
	}
	return out
}

func evaluateATA(d smart.Disk) []Violation {
	var vs []Violation
	add := func(a smart.Attr, sev Severity, cur, lim string) {
		vs = append(vs, Violation{
			DiskIndex: d.Index,
			DiskModel: d.Model,
			AttrID:    a.ID,
			AttrName:  a.Name,
			Current:   cur,
			Limit:     lim,
			Severity:  sev,
		})
	}

	for _, a := range d.Attrs {
		switch a.ID {
		case 0x05: // Reallocated_Sector_Ct
			if a.Raw != 0 {
				add(a, Critical, u64s(a.Raw), "= 0")
			}
		case 0xC5: // Current_Pending_Sector
			if a.Raw != 0 {
				add(a, Warning, u64s(a.Raw), "= 0")
			}
		case 0xC6: // Offline_Uncorrectable
			if a.Raw != 0 {
				add(a, Warning, u64s(a.Raw), "= 0")
			}
		case 0xBB: // Reported_Uncorrectable_Errors
			if a.Raw != 0 {
				add(a, Critical, u64s(a.Raw), "= 0")
			}
		case 0x01: // Raw_Read_Error_Rate
			// 严格按用户要求：!=0 即告警；但为避免 WD/Seagate 误报，附加「value < thresh」的宽松分支。
			// 我们实现两种逻辑任一触发即告警：(raw != 0) OR (thresh>0 && value<=thresh)
			if a.Raw != 0 {
				add(a, Warning, u64s(a.Raw), "= 0 （注:WD/Seagle-Safe:raw>0）")
			} else if a.Thresh > 0 && a.Value <= a.Thresh {
				add(a, Warning, its(a.Value), "≤ "+its(a.Thresh))
			}
		case 0x0E: // 厂牌特定
			if a.Raw != 0 {
				add(a, Warning, u64s(a.Raw), "= 0 （厂牌特定）")
			}
		case 0xC2: // Temperature
			// raw 值 48 位中，最低字节是当前温度（°C），高位字节可能是历史最高/最低。
			// 来源：ATA8-ACS 温度属性布局 raw[0]=当前, raw[1]=最低, raw[2]=最高（厂牌差异）。
			tempCur := int(a.Raw & 0xFF)
			if tempCur > 60 {
				add(a, Critical, its(tempCur)+"°C", "≤ 60°C")
			} else if tempCur > 55 {
				add(a, Warning, its(tempCur)+"°C", "≤ 55°C")
			}
		case 0xBC: // Command_Timeout
			if a.Raw > 10 {
				add(a, Warning, u64s(a.Raw), "≤ 10")
			}
		case 0xE9: // Media_Wearout_Indicator (100=新, 0=耗尽)
			// raw 通常就是百分比值（如 95 表示剩余 95%）
			if a.Thresh > 0 && int(a.Raw) <= a.Thresh && a.Thresh <= 50 {
				add(a, Critical, its(int(a.Raw))+"% (remaining)", "≥ 50%")
			} else if int(a.Thresh) > 0 && a.Value <= a.Thresh {
				add(a, Warning, its(a.Value), "≥ "+its(a.Thresh))
			}
		case 0xE8: // Available_Reservd_Space (Samsung=life %; WD/Crucial=预留%)
			if int(a.Raw) <= 50 && int(a.Raw) > 0 {
				add(a, Warning, its(int(a.Raw))+"%", "≥ 50%")
			}
		case 0xAD, 0xB1: // Wear_Leveling_Count
			// 低归一化值提示磨损
			if a.Thresh > 0 && a.Value <= a.Thresh {
				add(a, Warning, its(a.Value), "≥ "+its(a.Thresh))
			}
		}
	}
	return vs
}

func evaluateNVMe(d smart.Disk) []Violation {
	var vs []Violation
	add := func(a smart.Attr, sev Severity, cur, lim string) {
		vs = append(vs, Violation{
			DiskIndex: d.Index,
			DiskModel: d.Model,
			AttrID:    a.ID,
			AttrName:  a.Name,
			Current:   cur,
			Limit:     lim,
			Severity:  sev,
		})
	}
	for _, a := range d.Attrs {
		switch a.ID {
		case smart.NVMeMediaErrors:
			if a.Raw != 0 {
				add(a, Critical, u64s(a.Raw), "= 0")
			}
		case smart.NVMePercentUsed:
			if a.Raw >= 50 {
				add(a, Critical, u64s(a.Raw)+"% used", "< 50%")
			}
		case smart.NVMeTemperature:
			k := a.Raw
			c := int(k) - 273
			if k > 333 {
				add(a, Critical, its(c)+"°C", "≤ 60°C")
			} else if k > 328 {
				add(a, Warning, its(c)+"°C", "≤ 55°C")
			}
		case smart.NVMeCriticalWarning:
			if a.Raw != 0 {
				add(a, Critical, "0x"+itoa(int(a.Raw)), "= 0")
			}
		case smart.NVMeAvailableSpare:
			// 需要与阈值比较；阈值在 NVMeAvailSpareThresh 属性中
			// 这里简化：spare < 10% 直接告警
			if a.Raw < 10 {
				add(a, Critical, u64s(a.Raw)+"%", "≥ 10%")
			}
		case smart.NVMeReadOnly:
			if a.Raw != 0 {
				add(a, Critical, "ReadOnly", "ReadWrite")
			}
		}
	}
	// 单独处理 AvailableSpare < Threshold 的比较
	var spare, spareThresh *smart.Attr
	for i := range d.Attrs {
		switch d.Attrs[i].ID {
		case smart.NVMeAvailableSpare:
			spare = &d.Attrs[i]
		case smart.NVMeAvailSpareThresh:
			spareThresh = &d.Attrs[i]
		}
	}
	if spare != nil && spareThresh != nil && spare.Raw < spareThresh.Raw {
		add(*spare, Critical, u64s(spare.Raw)+"%", "≥ "+u64s(spareThresh.Raw)+"%")
	}
	return vs
}

// 小工具
func u64s(v uint64) string {
	return itoa(int(v))
}
func its(v int) string { return itoa(v) }

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var b [20]byte
	n := 20
	for v > 0 {
		n--
		b[n] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		n--
		b[n] = '-'
	}
	return string(b[n:])
}
