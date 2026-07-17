package main

import (
	"testing"

	"smonitor/smart"
)

func TestMergeFallbackDisksOnlyFillsMissingAttributes(t *testing.T) {
	primary := []smart.Disk{
		{Index: 0, Kind: smart.KindATA, Model: "Primary", Attrs: []smart.Attr{{ID: 5}}},
		{Index: 1, Kind: smart.KindATA, Model: "Missing"},
	}
	fallback := []smart.Disk{
		{Index: 0, Kind: smart.KindATA, Model: "Fallback", Attrs: []smart.Attr{{ID: 1}}},
		{Index: 1, Kind: smart.KindATA, Model: "Fallback", Attrs: []smart.Attr{{ID: 5}}},
		{Index: 2, Kind: smart.KindATA, Model: "Only WMI", Attrs: []smart.Attr{{ID: 9}}},
	}

	got := mergeFallbackDisks(primary, fallback)
	if len(got) != 3 || got[0].Attrs[0].ID != 5 || got[1].Attrs[0].ID != 5 || got[2].Index != 2 {
		t.Fatalf("unexpected merged disks: %+v", got)
	}
}
