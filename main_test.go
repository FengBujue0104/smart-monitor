package main

import (
	"os"
	"path/filepath"
	"testing"

	"smonitor/smart"
)

// 日志超过上限时归档为 .old 再重写；小日志不归档。
func TestOpenLogWithRotationArchivesOversizedLog(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "smonitor.log")

	// 超过 5MB 的旧日志 → 归档为 .old，新文件从空开始
	big := make([]byte, maxLogBytes+1)
	if err := os.WriteFile(path, big, 0644); err != nil {
		t.Fatal(err)
	}
	f, err := openLogWithRotation(path)
	if err != nil {
		t.Fatalf("open oversized log: %v", err)
	}
	f.Close()
	if _, err := os.Stat(path + ".old"); err != nil {
		t.Fatalf("oversized log was not archived: %v", err)
	}
	if fi, err := os.Stat(path); err != nil || fi.Size() != 0 {
		t.Fatalf("new log should start empty: size=%v err=%v", fi.Size(), err)
	}

	// 小日志 → 不归档，追加保持
	os.Remove(path + ".old")
	if err := os.WriteFile(path, []byte("small"), 0644); err != nil {
		t.Fatal(err)
	}
	f, err = openLogWithRotation(path)
	if err != nil {
		t.Fatalf("open small log: %v", err)
	}
	f.Close()
	if _, err := os.Stat(path + ".old"); !os.IsNotExist(err) {
		t.Fatalf("small log must not be archived: %v", err)
	}
	if fi, _ := os.Stat(path); fi.Size() != 5 {
		t.Fatalf("small log was truncated: size=%d", fi.Size())
	}
}

func TestMergeFallbackDisksOnlyFillsMissingAttributes(t *testing.T) {
	primary := []smart.Disk{
		{Index: 0, Kind: smart.KindATA, Model: "Primary", SmartStatusKnown: true, SmartStatusPassed: true, Attrs: []smart.Attr{{ID: 5}}},
		{Index: 1, Kind: smart.KindATA, Model: "Missing"},
	}
	fallback := []smart.Disk{
		{Index: 0, Kind: smart.KindATA, Model: "Fallback", SmartStatusKnown: true, SmartStatusPassed: false, Attrs: []smart.Attr{{ID: 1}}},
		{Index: 1, Kind: smart.KindATA, Model: "Fallback", SmartStatusKnown: true, SmartStatusPassed: false, Attrs: []smart.Attr{{ID: 5}}},
		{Index: 2, Kind: smart.KindATA, Model: "Only WMI", Attrs: []smart.Attr{{ID: 9}}},
	}

	got := smart.MergeFallbackDisks(primary, fallback)
	if len(got) != 3 || got[0].Attrs[0].ID != 5 || got[1].Attrs[0].ID != 5 || got[2].Index != 2 {
		t.Fatalf("unexpected merged disks: %+v", got)
	}
	if !got[0].SmartStatusKnown || !got[0].SmartStatusPassed {
		t.Fatalf("primary SMART status was overwritten: %+v", got[0])
	}
	if !got[1].SmartStatusKnown || got[1].SmartStatusPassed {
		t.Fatalf("fallback SMART status was not copied: %+v", got[1])
	}
}

func TestMergeFallbackDisksReplacesCorruptATASMARTData(t *testing.T) {
	primary := []smart.Disk{{
		Index: 0, Kind: smart.KindATA, Attrs: []smart.Attr{{ID: 1}},
		SMARTChecksumKnown: true, SMARTChecksumValid: false,
	}}
	fallback := []smart.Disk{{
		Index: 0, Kind: smart.KindATA, Attrs: []smart.Attr{{ID: 5}},
		SMARTChecksumKnown: true, SMARTChecksumValid: true,
	}}

	got := smart.MergeFallbackDisks(primary, fallback)
	if len(got) != 1 || len(got[0].Attrs) != 1 || got[0].Attrs[0].ID != 5 || !got[0].SMARTChecksumValid {
		t.Fatalf("expected valid fallback SMART data: %+v", got)
	}
}

func TestMergeFallbackDisksKeepsSuccessfulTransport(t *testing.T) {
	primary := []smart.Disk{{Index: 0, Kind: smart.KindATA, SMARTTransport: "ATA IOCTL", Attrs: []smart.Attr{{ID: 5}}}}
	fallback := []smart.Disk{{Index: 0, Kind: smart.KindATA, SMARTTransport: "WMI fallback", Attrs: []smart.Attr{{ID: 1}}}}
	got := smart.MergeFallbackDisks(primary, fallback)
	if got[0].SMARTTransport != "ATA IOCTL" || got[0].Attrs[0].ID != 5 {
		t.Fatalf("valid native transport/data was replaced: %+v", got[0])
	}

	primary[0].Attrs = nil
	got = smart.MergeFallbackDisks(primary, fallback)
	if got[0].SMARTTransport != "WMI fallback" || got[0].Attrs[0].ID != 1 {
		t.Fatalf("fallback transport/data was not used: %+v", got[0])
	}
}

func TestMergeFallbackDisksCorrectsUnprobedUSBNVMeKind(t *testing.T) {
	primary := []smart.Disk{{Index: 2, Kind: smart.KindATA, Model: "USB SSD", SMARTReadError: "ATA IOCTL failed"}}
	fallback := []smart.Disk{{Index: 2, Kind: smart.KindNVMe, Model: "NVMe SSD USB Device", SMARTReadError: "WMI fallback does not expose an NVMe Health Log"}}
	got := smart.MergeFallbackDisks(primary, fallback)
	if len(got) != 1 || got[0].Kind != smart.KindNVMe || got[0].SMARTReadError != fallback[0].SMARTReadError {
		t.Fatalf("USB NVMe classification was not corrected: %+v", got)
	}
}
