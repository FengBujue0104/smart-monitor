package smart

import "testing"

func TestSwapUTF16BytesPairsBytesAndKeepsOddTail(t *testing.T) {
	// 偶数长度：两两交换
	if got := swapUTF16Bytes([]byte{0x31, 0x32, 0x33, 0x34}); string(got) != "2143" {
		t.Fatalf("swap = %q, want 2143", got)
	}
	// 奇数长度：末尾单字节原样保留（不越界、不清零）
	if got := swapUTF16Bytes([]byte{0x31, 0x32, 0x33}); string(got) != "213" {
		t.Fatalf("swap odd = %q, want 213", got)
	}
	// 空输入
	if got := swapUTF16Bytes(nil); len(got) != 0 {
		t.Fatalf("swap empty = %q", got)
	}
	// 单字节
	if got := swapUTF16Bytes([]byte{0x41}); string(got) != "A" {
		t.Fatalf("swap single = %q", got)
	}
}

func TestParseNVMeHealthLogToleratesTruncatedPage(t *testing.T) {
	// 截断页（0x07..0xC7 之间）：核心健康字段齐全即可解析，缺失的可选
	// 计数器/温度时间按边界跳过，不能整页丢弃导致盘显示“SMART 未读取”。
	data := make([]byte, 0x40)
	data[0] = 0x02 // CriticalWarning
	data[1], data[2] = 0x2C, 0x01 // Temperature = 300K
	data[3], data[4], data[5] = 95, 10, 7 // spare / threshold / pct used
	data[0x20] = 100                       // DataUnitsRead 低位

	attrs := parseNVMeHealthLog(data)
	if attrs == nil {
		t.Fatal("truncated but core-complete NVMe log must parse")
	}
	values := map[int]uint64{}
	for _, a := range attrs {
		values[a.ID] = a.Raw
	}
	if values[NVMeCriticalWarning] != 0x02 || values[NVMeTemperature] != 300 ||
		values[NVMeAvailableSpare] != 95 || values[NVMeAvailSpareThresh] != 10 || values[NVMePercentUsed] != 7 {
		t.Fatalf("core fields lost on truncated page: %+v", values)
	}
	// 越界的可选字段为 0，不能触发告警
	if values[NVMeMediaErrors] != 0 {
		t.Fatalf("out-of-bounds media errors must be 0, got %d", values[NVMeMediaErrors])
	}
}

func TestParseNVMeHealthLogRejectsTinyLog(t *testing.T) {
	// 不足 0x07（连核心块都没有）时必须整体拒绝。
	if got := parseNVMeHealthLog(make([]byte, 0x06)); got != nil {
		t.Fatalf("tiny NVMe log parsed as %d attrs, want nil", len(got))
	}
}

func TestParseSMARTDataHandlesShortBuffer(t *testing.T) {
	// 少于 2 字节：无法判断版本头，直接返回 nil。
	if got := parseSMARTData([]byte{0x00}); got != nil {
		t.Fatalf("1-byte buffer parsed as %d attrs, want nil", len(got))
	}
	// 空输入
	if got := parseSMARTData(nil); got != nil {
		t.Fatalf("empty buffer parsed as %d attrs, want nil", len(got))
	}
}

func TestParseSMARTDataStopsAtBufferEnd(t *testing.T) {
	// 恰好容纳 2 字节头 + 1 个属性：解析 1 条，不越界。
	data := []byte{
		0x10, 0x00,
		0x09, 0x32, 0x00, 0x64, 0x64, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	}
	attrs := parseSMARTData(data)
	if len(attrs) != 1 || attrs[0].ID != 0x09 {
		t.Fatalf("short table parsed as %+v, want one 0x09 attr", attrs)
	}
}

func TestParseSMARTDataSkipsIncompleteAttribute(t *testing.T) {
	// 属性槽不足 12 字节（off+12 > len）时整槽跳过，不越界。
	data := []byte{
		0x00, 0x00,
		0x05, 0x32, 0x00, 0x64, 0x64, 0x01, 0x02, 0x03, 0x04, 0x05, // 10 字节，缺 2 字节
	}
	attrs := parseSMARTData(data)
	if len(attrs) != 0 {
		t.Fatalf("incomplete attribute parsed as %+v, want none", attrs)
	}
}
