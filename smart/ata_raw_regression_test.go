package smart

import "testing"

// 回归测试：用 CrystalDiskInfo_20260718090515.txt 中的真实 SMART_READ_DATA
// 字节验证 48 位 raw 解析（旧代码第 6 字节用 <<48 会放大 256 倍）。
//
// WD Blue SA510 属性表字节（报告 0x000 行）：
//
//	00 00 | 05 32 00 64 64 00 00 00 00 00 00 00 | 09 32 00 64 64 3E 12 00 ...
//
// 第 7 个属性（index 6, off=2+6*12=74）：A5 32 00 64 64 37 08 3E 08 E8 04 00
// raw 小端 = 37 08 3E 08 E8 04 -> 0x04E8083E0837（CrystalDiskInfo 显示 04E8083E0837）。
func TestParseSMARTDataRaw48BitMatchesCrystalDiskInfo(t *testing.T) {
	// 从 CrystalDiskInfo 文本中逐字节构造 WD Blue SA510 的 SMART_READ_DATA 前 4 行（0x000-0x03F）。
	// 第 0 字节 0x00 + 第 1 字节 0x00 构成版本头（CrystalDiskInfo 此盘无 0x10 版本头，
	// smartTableBase 会启发式选择 base=2）。
	data := []byte{
		0x00, 0x00,
		0x05, 0x32, 0x00, 0x64, 0x64, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x09, 0x32, 0x00, 0x64, 0x64, 0x3E, 0x12, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x0C, 0x32, 0x00, 0x64, 0x64, 0x68, 0x10, 0x00, 0x00, 0x00, 0x00, 0x00,
		0xA5, 0x32, 0x00, 0x64, 0x64, 0x37, 0x08, 0x3E, 0x08, 0xE8, 0x04, 0x00,
		0xA6, 0x32, 0x00, 0x64, 0x64, 0x05, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0xA7, 0x32, 0x00, 0x64, 0x64, 0x11, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	}
	// 补齐 512 字节（剩余为 0）
	data = append(data, make([]byte, 512-len(data))...)

	attrs := parseSMARTData(data)
	byID := map[int]Attr{}
	for _, a := range attrs {
		byID[a.ID] = a
	}

	tests := []struct {
		id   int
		want uint64 // CrystalDiskInfo RawValues(6)
	}{
		{0x05, 0x000000000000}, // 000000000000
		{0x09, 0x00000000123E}, // 通电时间 0x123E=4670 小时
		{0x0C, 0x000000001068}, // 通电次数 0x1068=4200
		{0xA5, 0x04E8083E0837}, // 块擦除计数 (SLC) —— 第 6 字节非零，验证 <<40
		{0xA6, 0x000000000005}, // 最小 PE 次数
		{0xA7, 0x000000000011}, // 单个裸片最大坏块数
	}
	for _, tt := range tests {
		a, ok := byID[tt.id]
		if !ok {
			t.Fatalf("attribute 0x%02X not parsed", tt.id)
		}
		if a.Raw != tt.want {
			t.Errorf("0x%02X raw = 0x%X, want 0x%X (raw<<48 位移 bug 会使 0xA5 放大 256 倍)", tt.id, a.Raw, tt.want)
		}
		if a.Value != 100 || a.Worst != 100 {
			t.Errorf("0x%02X value/worst = %d/%d, want 100/100", tt.id, a.Value, a.Worst)
		}
	}
}

// 回归测试：KIOXIA C2 温度 raw 的 48 位解析（报告显示 raw=002C00130022，当前温度 0x22=34°C）。
func TestParseSMARTDataKioxiaTemperatureRaw(t *testing.T) {
	data := make([]byte, 512)
	// 2 字节版本头 0x10 0x00
	data[0], data[1] = 0x10, 0x00
	// 第 1 属性 off=2: 09 12 00 64 64 96 78 00 00 00 00 00 -> raw=0x7896=30870
	copy(data[2:14], []byte{0x09, 0x12, 0x00, 0x64, 0x64, 0x96, 0x78, 0x00, 0x00, 0x00, 0x00, 0x00})
	// 第 8 属性 off=2+7*12=86: C2 23 00 42 38 22 00 13 00 2C 00 00 -> raw=0x2C00130022
	copy(data[86:98], []byte{0xC2, 0x23, 0x00, 0x42, 0x38, 0x22, 0x00, 0x13, 0x00, 0x2C, 0x00, 0x00})

	attrs := parseSMARTData(data)
	byID := map[int]Attr{}
	for _, a := range attrs {
		byID[a.ID] = a
	}
	if a, ok := byID[0x09]; !ok || a.Raw != 0x7896 {
		t.Fatalf("0x09 raw = %#v, want 0x7896 (30870 小时)", a.Raw)
	}
	if a, ok := byID[0xC2]; !ok || a.Raw != 0x2C00130022 {
		t.Fatalf("0xC2 raw = 0x%X, want 0x2C00130022", a.Raw)
	}
	// 温度当前值 = 最低字节
	if a, ok := byID[0xC2]; ok && a.Raw&0xFF != 0x22 {
		t.Fatalf("0xC2 当前温度 = 0x%02X, want 0x22 (34°C)", a.Raw&0xFF)
	}
}
