package smart

import (
	"encoding/binary"
	"fmt"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// 让 deviceHandle 内部使用 windows.Handle
type windowsHandle = windows.Handle

// STORAGE_PROPERTY_QUERY 用于 IOCTL_STORAGE_QUERY_PROPERTY。
// 布局：
//
//	Offset  Size  Field
//	0x00    4     PropertyId（StorageDeviceProperty=0 / StorageAdapterProperty=1）
//	0x04    4     QueryType（PropertyStandardQuery=0）
//	0x08    1     AdditionalParameters[0]（PropertyExistsQuery 用）
type storagePropertyQuery struct {
	PropertyID uint32
	QueryType  uint32
	Additional byte
	_          [3]byte // padding
}

// STORAGE_DEVICE_DESCRIPTOR 中我们关心的字段（来自 IOCTL_STORAGE_QUERY_PROPERTY 输出）。
// 实际是一个变长结构：头 + VendorID/ProductID/ProductRevision/SerialNumber 偏移 + 字符串。
// 头部长 36 字节（Windows SDK），随后 4 个偏移，再 1 字节 DeviceType/DeviceTypeModifier，+ 1 字节 Removable/CommandQueueing，再 + 字符串。
const (
	StorageDeviceProperty      = 0
	StorageAdapterProperty     = 1
	PropertyStandardQuery      = 0
	IOCTL_DISK_GET_LENGTH_INFO = 0x0007405C
	// Windows exposes physical disks as \\.\PhysicalDriveN. 32 is too small
	// for workstations with HBAs, storage pools, or attached enclosures.
	maxPhysDrives = 256
)

// queryStorageDevice 发 IOCTL_STORAGE_QUERY_PROPERTY，返回原始描述符字节。
func queryStorageDevice(h windows.Handle) ([]byte, error) {
	q := storagePropertyQuery{
		PropertyID: StorageDeviceProperty,
		QueryType:  PropertyStandardQuery,
	}
	out := make([]byte, 4096)
	var returned uint32
	err := windows.DeviceIoControl(
		h,
		IOCTL_STORAGE_QUERY_PROPERTY,
		(*byte)(unsafe.Pointer(&q)),
		uint32(unsafe.Sizeof(q)),
		&out[0],
		uint32(len(out)),
		&returned,
		nil,
	)
	if err != nil {
		return nil, err
	}
	return out[:returned], nil
}

func queryDiskSizeGB(h windows.Handle) float64 {
	out := make([]byte, 8)
	var returned uint32
	if err := windows.DeviceIoControl(h, IOCTL_DISK_GET_LENGTH_INFO, nil, 0,
		&out[0], uint32(len(out)), &returned, nil); err != nil || returned < 8 {
		return 0
	}
	bytes := binary.LittleEndian.Uint64(out)
	return float64(bytes) / (1024 * 1024 * 1024)
}

// parseStorageDescriptor 从 STORAGE_DEVICE_DESCRIPTOR 输出中解析型号/序列号/固件/容量。
// 典型字段布局（Windows SDK，小端）：
//
//	0x00: Version(4)
//	0x04: Size(4)
//	0x08: DeviceType(1)
//	0x09: DeviceTypeModifier(1)
//	0x0A: RemovableMedia(1)
//	0x0B: CommandQueueing(1)
//	0x0C: VendorIdOffset(4)（0 表示无）
//	0x10: ProductIdOffset(4)
//	0x14: ProductRevisionOffset(4)
//	0x18: SerialNumberOffset(4)
//	0x1C: BusType(STORAGE_BUS_TYPE: 0x11=NVMe 或 0x07=SATA 等)
//	0x20: RawPropertiesLength(4)
//	0x24: RawDeviceProperties[RawPropertiesLength]
func parseStorageDescriptor(b []byte) (vendor, product, revision, serial string, busType uint32) {
	if len(b) < 0x24 {
		return
	}
	vendorOff := binary.LittleEndian.Uint32(b[0x0C:0x10])
	productOff := binary.LittleEndian.Uint32(b[0x10:0x14])
	revisionOff := binary.LittleEndian.Uint32(b[0x14:0x18])
	serialOff := binary.LittleEndian.Uint32(b[0x18:0x1C])
	busType = binary.LittleEndian.Uint32(b[0x1C:0x20])

	readAt := func(off uint32) string {
		if off == 0 || int(off) >= len(b) {
			return ""
		}
		end := off
		for end < uint32(len(b)) && b[end] != 0 {
			end++
		}
		return strings.TrimSpace(string(b[off:end]))
	}
	vendor = readAt(vendorOff)
	product = readAt(productOff)
	revision = readAt(revisionOff)
	serial = readAt(serialOff)
	return
}

// OpenDeviceForTest 是 openDevice 的导出版（供 main 包做「是否管理员」探测用）。
func OpenDeviceForTest(path string) (closer interface{ Close() error }, err error) {
	h, err := openDevice(path)
	if err != nil {
		return nil, err
	}
	return &deviceHandle{h}, nil
}

type deviceHandle struct{ h windowsHandle }

func (d *deviceHandle) Close() error { return windows.CloseHandle(d.h) }

// Discover 枚举 PhysicalDrive 0..maxPhysDrives-1，返回每块盘的统一 Disk（仅元数据 + 已采集的健康属性）。
// 采集顺序：
//  1. IOCTL_STORAGE_QUERY_PROPERTY 获取型号/序列/固件/总线类型。
//  2. 若 BusType=NVMe -> ReadNVMeHealth()。
//  3. 否则 (SATA/ATA) -> ReadIdentify + ReadSMARTData + ReadSMARTThresholds
//
// 每一步失败则降级：仍返回 Disk（Attrs 可能为空或仅 NVMe 健康属性）。
func Discover() ([]Disk, error) {
	var disks []Disk

	for i := 0; i < maxPhysDrives; i++ {
		path := fmt.Sprintf(`\\.\PhysicalDrive%d`, i)
		h, err := openDevice(path)
		if err != nil {
			// ERROR_FILE_NOT_FOUND / ERROR_ACCESS_DENIED 常见；跳过
			continue
		}

		desc, err := queryStorageDevice(h)
		if err != nil {
			windows.CloseHandle(h)
			continue
		}
		vendor, product, revision, serial, busType := parseStorageDescriptor(desc)
		model := product
		if vendor != "" {
			model = vendor + " " + product
		}

		d := Disk{
			Index:    i,
			Path:     path,
			Model:    model,
			Serial:   serial,
			Firmware: revision,
		}
		d.SizeGB = queryDiskSizeGB(h)

		switch {
		case busType == 0x11: // NVMe
			d.Kind = KindNVMe
			attrs, err := ReadNVMeHealth(h)
			if err != nil {
				// 降级：仍返回空 Attrs 的盘
				d.Kind = KindNVMe
			}
			d.Attrs = attrs
		default:
			d.Kind = KindATA
			// 型号/序列号/固件已由 IOCTL_STORAGE_QUERY_PROPERTY 在 desc 中解析；
			// 不再通过 SMART_RCV_DRIVE_DATA 发 IDENTIFY（某些控制器返回异常）。
			// 若 STORAGE_QUERY 未拿到型号，再尝试 IDENTIFY 兜底。
			if d.Model == "" || d.Model == "Unknown" {
				m, s, fw, idErr := ReadIdentify(h, byte(i))
				if idErr != nil {
					fmt.Printf("[disk%d] IDENTIFY_ERR: %v\n", i, idErr)
				} else if m != "" {
					d.Model = m
					d.Serial = s
					d.Firmware = fw
				}
			}
			attrs, _, checksumValid, smErr := ReadSMARTDataDetailed(h, byte(i))
			status, statusErr := ReadSMARTOverallStatus(h)
			if statusErr == nil {
				d.SmartStatusKnown = true
				d.SmartStatusPassed = status
			} else {
				fmt.Printf("[disk%d] SMART_STATUS_ERR: %v\n", i, statusErr)
			}
			if smErr != nil {
				fmt.Printf("[disk%d] SMART_DATA_ERR: %v\n", i, smErr)
			} else {
				d.SMARTChecksumKnown = true
				d.SMARTChecksumValid = checksumValid
				th, thErr := ReadSMARTThresholds(h, byte(i))
				if thErr == nil {
					for i := range attrs {
						if t, ok := th[attrs[i].ID]; ok {
							attrs[i].Thresh = t
						}
					}
				}
				for i := range attrs {
					attrs[i].Name = ATAAttrNameForModel(d.Model, attrs[i].ID)
				}
				d.Attrs = attrs
			}
		}

		windows.CloseHandle(h)
		disks = append(disks, d)
	}
	return disks, nil
}
