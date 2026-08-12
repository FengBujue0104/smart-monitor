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

func TestParseNVMeHealthLogRejectsTruncatedLog(t *testing.T) {
	// 日志短于 0xC8 字节（复合温度时间字段末尾）时必须整体拒绝，
	// 不能返回部分解析结果误导为完整健康数据。
	if got := parseNVMeHealthLog(make([]byte, 0xC7)); got != nil {
		t.Fatalf("truncated NVMe log parsed as %d attrs, want nil", len(got))
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
