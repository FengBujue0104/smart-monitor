package smart

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strings"
	"unicode"

	"github.com/yusufpapurcu/wmi"
)

// ===== WMI SMART 采集（root\WMI / MSStorageDriver_FailurePredictData）=====
// 这是 Windows 原生 SMART WMI 提供程序，兼容性最好，不需要特殊 IOCTL。
// 来源：MSDN MSStorageDriver_FailurePredictData class。

// wmiFailurePredictData 对应 MSStorageDriver_FailurePredictData 的字段。
type wmiFailurePredictData struct {
	InstanceName   string // 关联键，如 "SCSI\\Disk&Ven_NVMe&Prod_Samsung..."
	VendorSpecific []byte // safearray of uint8：2 字节版本头 + 30×12 字节属性
}

// wmiFailurePredictThresholds 对应 MSStorageDriver_FailurePredictThresholds。
type wmiFailurePredictThresholds struct {
	InstanceName   string
	VendorSpecific []byte // 2 字节头 + 30×12 字节，每记录 byte[1]=阈值
}

type wmiFailurePredictStatus struct {
	InstanceName   string
	PredictFailure bool
}

// wmiDiskDrive 对应 Win32_DiskDrive。
type wmiDiskDrive struct {
	Index            uint32
	Model            string
	SerialNumber     string
	FirmwareRevision string
	Size             uint64
	MediaType        string
	InterfaceType    string // IDE/SCSI/USB...
	PNPDeviceID      string
}

// DiscoverWMI 通过 WMI 枚举磁盘并读取可用的 SMART 属性。
// 即使 root\WMI SMART 类不可用，只要 Win32_DiskDrive 可查询，仍返回磁盘元数据。
func DiscoverWMI() ([]Disk, error) {
	// 1. 读 Win32_DiskDrive 获取型号/序列/固件/容量
	var drives []wmiDiskDrive
	q := wmi.CreateQuery(&drives, "", "Win32_DiskDrive")
	err := wmi.Query(q, &drives)
	if err != nil {
		return nil, fmt.Errorf("Win32_DiskDrive query: %w", err)
	}

	// 2. 读 SMART 数据
	var smartData []wmiFailurePredictData
	q2 := wmi.CreateQuery(&smartData, "", "MSStorageDriver_FailurePredictData")
	_ = wmi.QueryNamespace(q2, &smartData, `root\WMI`) // 只影响属性，不应丢弃已枚举磁盘

	// 3. 读阈值
	var thresholds []wmiFailurePredictThresholds
	q3 := wmi.CreateQuery(&thresholds, "", "MSStorageDriver_FailurePredictThresholds")
	_ = wmi.QueryNamespace(q3, &thresholds, `root\WMI`) // 阈值可选，失败不阻塞

	// 4. 读取 WMI 提供的 SMART overall failure 状态。
	var statuses []wmiFailurePredictStatus
	q4 := wmi.CreateQuery(&statuses, "", "MSStorageDriver_FailurePredictStatus")
	_ = wmi.QueryNamespace(q4, &statuses, `root\WMI`) // 部分控制器没有该类，状态可未知

	// 建立 InstanceName -> data 映射
	dataMap := map[string][]byte{}
	for _, d := range smartData {
		dataMap[d.InstanceName] = d.VendorSpecific
	}
	threshMap := map[string][]byte{}
	for _, t := range thresholds {
		threshMap[t.InstanceName] = t.VendorSpecific
	}
	statusMap := map[string]bool{}
	for _, s := range statuses {
		statusMap[s.InstanceName] = s.PredictFailure
	}

	var disks []Disk
	for _, drv := range drives {
		d := Disk{
			Index:    int(drv.Index),
			Model:    cleanWMIString(drv.Model),
			Serial:   cleanWMIString(drv.SerialNumber),
			Firmware: cleanWMIString(drv.FirmwareRevision),
			SizeGB:   float64(drv.Size) / (1024 * 1024 * 1024),
		}
		// 通过 InstanceName 关联：Win32_DiskDrive.DeviceID 与 FailurePredictData.InstanceName 含厂商信息
		// 简化：按 Index 顺序匹配（InstanceName 通常按磁盘顺序）
		// 更可靠：用 Model+Serial 子串匹配
		data, instanceName := findDataForDrive(dataMap, drv)
		d.Kind = classifyWMIDisk(drv, instanceName)
		if predictFailure, ok := findStatusForDrive(statusMap, drv); ok {
			d.SmartStatusKnown = true
			d.SmartStatusPassed = !predictFailure
		}
		if d.Kind == KindATA && len(data) >= 14 {
			d.SMARTTransport = "WMI fallback"
			th, ok := findDataForDriveOK(threshMap, drv)
			if !ok {
				th = nil
			}
			applyWMIATAData(&d, data, th)
		}
		disks = append(disks, d)
	}
	return disks, nil
}

func classifyWMIDisk(drv wmiDiskDrive, instanceName string) DiskKind {
	identity := strings.ToLower(drv.InterfaceType + " " + drv.PNPDeviceID + " " + instanceName)
	if strings.Contains(identity, "nvme") {
		return KindNVMe
	}
	return KindATA // USB/SCSI 桥接盘的 WMI SMART 表沿用 ATA 格式
}

func applyWMIATAData(d *Disk, data, thresholds []byte) {
	attrs := parseWMIAttributes(data)
	thresholdsValid := true
	if len(thresholds) >= 512 {
		d.SMARTThresholdChecksumKnown = true
		thresholdsValid = smartChecksumValid(thresholds)
		d.SMARTThresholdChecksumValid = thresholdsValid
	}
	if len(thresholds) >= 14 && thresholdsValid {
		applyThresholds(attrs, thresholds)
	}
	for i := range attrs {
		attrs[i].Name = ATAAttrNameForModel(d.Model, attrs[i].ID)
	}
	d.Attrs = attrs
	if len(data) >= 512 {
		d.SMARTChecksumKnown = true
		d.SMARTChecksumValid = smartChecksumValid(data)
	}
}

func findStatusForDrive(m map[string]bool, drv wmiDiskDrive) (bool, bool) {
	pnp := normalizeWMIKey(drv.PNPDeviceID)
	model := normalizeWMIKey(drv.Model)
	serial := normalizeWMIKey(drv.SerialNumber)
	var modelMatches []bool
	for name, predictFailure := range m {
		n := normalizeWMIKey(name)
		if pnp != "" && strings.Contains(n, pnp) {
			return predictFailure, true
		}
		if serial != "" && wmiIdentityMatch(n, drv.SerialNumber) {
			return predictFailure, true
		}
		if model != "" && wmiIdentityMatch(n, drv.Model) {
			modelMatches = append(modelMatches, predictFailure)
		}
	}
	if len(modelMatches) == 1 {
		return modelMatches[0], true
	}
	return false, false
}

// findDataForDriveOK 返回 ([]byte, bool)。
func findDataForDriveOK(m map[string][]byte, drv wmiDiskDrive) ([]byte, bool) {
	d, _ := findDataForDrive(m, drv)
	return d, d != nil
}

// findDataForDrive 根据型号/序列号子串匹配 WMI InstanceName。
func findDataForDrive(m map[string][]byte, drv wmiDiskDrive) ([]byte, string) {
	// 优先用 PNPDeviceID/InstanceName 的设备路径关联，避免同型号磁盘串数据。
	pnp := normalizeWMIKey(drv.PNPDeviceID)
	model := normalizeWMIKey(drv.Model)
	serial := normalizeWMIKey(drv.SerialNumber)
	var modelMatches []struct {
		data []byte
		name string
	}
	for name, data := range m {
		if len(data) < 14 {
			continue
		}
		n := normalizeWMIKey(name)
		if pnp != "" && strings.Contains(n, pnp) {
			return data, name
		}
		if serial != "" && wmiIdentityMatch(n, drv.SerialNumber) {
			return data, name
		}
		if model != "" && wmiIdentityMatch(n, drv.Model) {
			modelMatches = append(modelMatches, struct {
				data []byte
				name string
			}{data: data, name: name})
		}
	}
	if len(modelMatches) == 1 {
		return modelMatches[0].data, modelMatches[0].name
	}
	// 关联失败时必须返回空，不能把另一块盘的数据伪装成本盘数据。
	return nil, ""
}

func normalizeWMIKey(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(cleanWMIString(s)) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func wmiIdentityMatch(normalizedName, identity string) bool {
	normalized := normalizeWMIKey(identity)
	if normalized == "" {
		return false
	}
	if strings.Contains(normalizedName, normalized) {
		return true
	}
	parts := strings.FieldsFunc(cleanWMIString(identity), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	if len(parts) < 2 {
		return false
	}
	for _, part := range parts {
		if !strings.Contains(normalizedName, normalizeWMIKey(part)) {
			return false
		}
	}
	return true
}

// parseWMIAttributes 解析 WMI VendorSpecific 字节为 Attr 列表。
// 布局：2 字节版本头 + 30×12 字节属性。
// 每属性 12 字节：[0]=ID, [1-2]=flags, [3]=value, [4]=worst, [5-10]=raw(48-bit LE), [11]=reserved
func parseWMIAttributes(data []byte) []Attr {
	if len(data) < 14 {
		return nil
	}
	var attrs []Attr
	for i := 0; i < 30; i++ {
		off := 2 + i*12
		if off+12 > len(data) {
			break
		}
		id := int(data[off])
		if id == 0 {
			continue
		}
		value := int(data[off+3])
		worst := int(data[off+4])
		flags := uint16(data[off+1]) | uint16(data[off+2])<<8
		raw := uint64(data[off+5]) | uint64(data[off+6])<<8 | uint64(data[off+7])<<16 |
			uint64(data[off+8])<<24 | uint64(data[off+9])<<32 | uint64(data[off+10])<<48
		attrs = append(attrs, Attr{
			ID:    id,
			Name:  ATAAttrName(id),
			Flags: flags,
			Raw:   raw,
			Value: value,
			Worst: worst,
			Kind:  "ata",
		})
	}
	return attrs
}

// applyThresholds 把阈值页数据合并到 attrs。
func applyThresholds(attrs []Attr, th []byte) {
	for i := 0; i < 30; i++ {
		off := 2 + i*12
		if off+12 > len(th) {
			break
		}
		id := int(th[off])
		// 阈值页布局与数据页相同：记录第 3 字节（off+3）为阈值
		for j := range attrs {
			if attrs[j].ID == id {
				attrs[j].Thresh = int(th[off+1])
				break
			}
		}
	}
}

// cleanWMIString 去掉 WMI 字符串尾部空字节和首尾空格。
func cleanWMIString(s string) string {
	s = strings.ReplaceAll(s, "\x00", "")
	return strings.TrimSpace(s)
}

// 小工具：binary 已用
var _ = binary.LittleEndian
var _ = bytes.TrimSpace
