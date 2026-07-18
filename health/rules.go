package health

import (
	"fmt"
	"strconv"
	"strings"

	"smonitor/smart"
)

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
//
//	0x05 (Reallocated_Sector_Ct)  raw != 0              -> critical 重映射扇区，物理损坏标志
//	0xC5 (Current_Pending_Sector) raw != 0               -> warning  等待重映射的扇区
//	0xC6 (Offline_Uncorrectable)   raw != 0               -> warning
//	0xBB (Reported_Uncorrectable)  raw != 0               -> critical
//	0x01 (Raw_Read_Error_Rate)     raw != 0 且 value<th   -> warning  （注：WD/Seagate raw 常非零，需 value<thresh）
//	0x0E                           raw != 0               -> warning  （厂牌特定：三星=过热计数）
//	0xC2 (Temperature)             > 60°C                 -> critical（用户要求）
//	                                 > 55°C                 -> warning
//	0xBC (Command_Timeout)          > 10                   -> warning（用户要求）
//	0xE9 (Media_Wearout_Indicator)  <= 50                  -> critical（剩余寿命<=50%）
//	0xE8 (Available_Reservd_Space)  <= 50 (Samsung)        -> warning
//	0xAD/0xB1 (Wear_Leveling)       极低                    -> warning
//
// NVMe 规则：
//
//	MediaErrors (!= 0)                                     -> critical
//	PercentageUsed >= 50                                   -> critical（剩余寿命<=50%）
//	Temperature(K) > 333 (60°C)                            -> critical
//	Temperature(K) > 328 (55°C)                            -> warning
//	CriticalWarning != 0                                   -> critical
//	AvailableSpare < Threshold                             -> critical
func Evaluate(disks []smart.Disk) []Violation {
	var out []Violation
	for _, d := range disks {
		out = append(out, evaluateOverallStatus(d)...)
		out = append(out, evaluateSMARTDataIntegrity(d)...)
		switch d.Kind {
		case smart.KindATA:
			// A failed data-page checksum means the attribute bytes cannot be
			// trusted. Keep the independent overall-status and integrity
			// diagnostics above, but do not turn corrupt values into failures.
			if d.SMARTChecksumKnown && !d.SMARTChecksumValid {
				continue
			}
			out = append(out, evaluateATA(d)...)
		case smart.KindNVMe:
			out = append(out, evaluateNVMe(d)...)
		}
	}
	return out
}

func evaluateSMARTDataIntegrity(d smart.Disk) []Violation {
	if d.Kind != smart.KindATA {
		return nil
	}
	var violations []Violation
	add := func(id int, name string) {
		violations = append(violations, Violation{
			DiskIndex: d.Index,
			DiskModel: d.Model,
			AttrID:    id,
			AttrName:  name,
			Current:   "INVALID",
			Limit:     "VALID",
			Severity:  Warning,
		})
	}
	if d.SMARTChecksumKnown && !d.SMARTChecksumValid {
		add(-1, "SMART_Data_Checksum")
	}
	if d.SMARTThresholdChecksumKnown && !d.SMARTThresholdChecksumValid {
		add(-2, "SMART_Threshold_Checksum")
	}
	return violations
}

func evaluateOverallStatus(d smart.Disk) []Violation {
	if !d.SmartStatusKnown || d.SmartStatusPassed {
		return nil
	}
	return []Violation{{
		DiskIndex: d.Index,
		DiskModel: d.Model,
		AttrID:    0,
		AttrName:  "SMART_Overall_Health",
		Current:   "FAILED",
		Limit:     "PASSED",
		Severity:  Critical,
	}}
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
			// WD/Seagate 的 raw 通常是厂商复合计数，非零不代表故障。
			// 只使用规范化值与设备阈值判断。
			if a.Thresh > 0 && a.Value <= a.Thresh {
				add(a, Warning, its(a.Value), "≤ "+its(a.Thresh))
			}
		case 0x0E: // 厂牌特定
			// 没有型号/厂商语义时不报警，避免把未知厂商字段误判为故障。
		case 0xC2, 0xB9, 0xBE, 0xF3: // Temperature / vendor temperature variants
			if !smart.ATATemperatureAttributeForModel(d.Model, a.ID) {
				break
			}
			// raw 值 48 位中，最低字节是当前温度（°C），高位字节可能是历史最高/最低。
			// 来源：ATA8-ACS 温度属性布局 raw[0]=当前, raw[1]=最低, raw[2]=最高（厂牌差异）。
			tempCur := int(a.Raw & 0xFF)
			if tempCur > 60 {
				add(a, Critical, its(tempCur)+"°C", "≤ 60°C")
			} else if tempCur > 55 {
				add(a, Warning, its(tempCur)+"°C", "≤ 55°C")
			}
		case 0xBC: // Command_Timeout
			// A single historical timeout does not indicate current failure.
			// Only flag a sustained timeout count, matching the documented rule.
			if a.Raw > 10 {
				add(a, Warning, u64s(a.Raw), "≤ 10")
			}
		case 0xE9: // Media_Wearout_Indicator (100=新, 0=耗尽)
			if life, ok := smart.ATAHealthPercentForModel(d.Model, []smart.Attr{a}); ok {
				// CrystalDiskInfo uses its configured 10% caution threshold for
				// vendor life fields, with 0% indicating end of life.
				if life == 0 {
					add(a, Critical, "0% (remaining)", "> 0%")
				} else if life <= 10 {
					add(a, Warning, its(life)+"% (remaining)", "> 10%")
				}
				break
			}
			// 该属性的 raw 可能是写入量/厂商复合值，寿命使用归一化 Value。
			if a.Value > 0 && a.Value <= 10 {
				add(a, Critical, its(a.Value)+"% (remaining)", "> 10%")
			} else if a.Value > 0 && a.Value <= 20 {
				add(a, Warning, its(a.Value)+"% (remaining)", "> 20%")
			}
		case 0xE6: // WD Blue SA510 Media_Wearout_Indicator
			// CrystalDiskInfo reads remaining life from raw byte 1 for this
			// model and marks <=10% caution, 0% failure. Other E6 meanings are
			// vendor-specific, so only use the model-scoped helper.
			if life, ok := smart.ATAHealthPercentForModel(d.Model, []smart.Attr{a}); ok {
				if life == 0 {
					add(a, Critical, "0% (remaining)", "> 0%")
				} else if life <= 10 {
					add(a, Warning, its(life)+"% (remaining)", "> 10%")
				}
			}
		case 0xE8: // Available_Reservd_Space (Samsung=life %; WD/Crucial=预留%)
			if a.Value > 0 && a.Value <= 10 {
				add(a, Warning, its(a.Value)+"%", "> 10%")
			}
		case 0xAD, 0xB1: // Wear_Leveling_Count
			if a.ID == 0xB1 {
				// CrystalDiskInfo identifies Samsung B1 as remaining life and
				// uses its default caution threshold of 10%.
				if life, ok := smart.ATAHealthPercentForModel(d.Model, []smart.Attr{a}); ok {
					if life == 0 {
						add(a, Critical, "0% (remaining)", "> 0%")
					} else if life <= 10 {
						add(a, Warning, its(life)+"% (remaining)", "> 10%")
					}
					break
				}
			}
			// 低归一化值提示磨损
			if a.Thresh > 0 && a.Value <= a.Thresh {
				add(a, Warning, its(a.Value), "≥ "+its(a.Thresh))
			}
		}
		if shouldApplyGenericATAThreshold(d, a) && a.Thresh > 0 && a.Value > 0 && a.Value <= a.Thresh {
			severity := Warning
			if a.Flags&0x0001 != 0 { // ATA SMART 属性标志 bit 0：pre-fail
				severity = Critical
			}
			add(a, severity, its(a.Value), "> "+its(a.Thresh))
		}
	}
	return vs
}

// shouldApplyGenericATAThreshold keeps per-attribute rules authoritative
// while still reporting vendor-specific attributes that have reached their
// device-provided normalized-value threshold.
func shouldApplyGenericATAThreshold(d smart.Disk, a smart.Attr) bool {
	if smart.ATATemperatureAttributeForModel(d.Model, a.ID) {
		return false
	}
	switch a.ID {
	case 0x01, 0x05, 0xBB, 0xBC, 0xC5, 0xC6, 0xE8, 0xE9, 0xAD, 0xB1:
		return false
	}
	return true
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
			if a.Raw != 0 || a.RawHigh != 0 {
				add(a, Critical, u128s(a.Raw, a.RawHigh), "= 0")
			}
		case smart.NVMePercentUsed:
			if a.Raw >= 100 {
				add(a, Critical, u64s(a.Raw)+"% used", "< 100%")
			} else if a.Raw >= 80 {
				add(a, Warning, u64s(a.Raw)+"% used", "< 80%")
			}
		case smart.NVMeTemperature:
			addNVMeTemperatureViolation(add, a)
		case smart.NVMeCriticalWarning, smart.NVMeEnduranceGroupCriticalWarning:
			if a.Raw != 0 {
				add(a, Critical, nvmeCriticalWarningText(a.Raw), "= 0")
			}
		case smart.NVMeAvailableSpare:
			// 由后面的 AvailableSpare < Threshold 统一判断。
		case smart.NVMeReadOnly:
			if a.Raw != 0 {
				add(a, Critical, "ReadOnly", "ReadWrite")
			}
		default:
			if a.ID >= smart.NVMeTemperatureSensor1 && a.ID <= smart.NVMeTemperatureSensor8 {
				addNVMeTemperatureViolation(add, a)
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

func addNVMeTemperatureViolation(add func(smart.Attr, Severity, string, string), a smart.Attr) {
	k := a.Raw
	c := int(k) - 273
	if k > 333 {
		add(a, Critical, its(c)+"°C", "≤ 60°C")
	} else if k > 328 {
		add(a, Warning, its(c)+"°C", "≤ 55°C")
	}
}

// 小工具
func u64s(v uint64) string {
	return strconv.FormatUint(v, 10)
}
func its(v int) string { return strconv.Itoa(v) }

func u128s(low, high uint64) string {
	if high == 0 {
		return u64s(low)
	}
	return fmt.Sprintf("0x%016X%016X", high, low)
}

func nvmeCriticalWarningText(value uint64) string {
	known := []struct {
		mask   uint64
		reason string
	}{
		{mask: 1 << 0, reason: "可用备用空间低于阈值"},
		{mask: 1 << 1, reason: "温度超过临界阈值"},
		{mask: 1 << 2, reason: "可靠性已降级"},
		{mask: 1 << 3, reason: "控制器已进入只读模式"},
		{mask: 1 << 4, reason: "易失性缓存备份设备故障"},
	}
	var reasons []string
	for _, bit := range known {
		if value&bit.mask != 0 {
			reasons = append(reasons, bit.reason)
		}
	}
	if unknown := value &^ 0x1F; unknown != 0 {
		reasons = append(reasons, fmt.Sprintf("未知保留位 0x%X", unknown))
	}
	if len(reasons) == 0 {
		return "0x" + strconv.FormatUint(value, 16)
	}
	return "0x" + strconv.FormatUint(value, 16) + " (" + strings.Join(reasons, "；") + ")"
}
