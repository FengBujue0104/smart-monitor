package smart

// DiscoverWithFallback applies the same compatibility strategy to every scan:
// native IOCTL first, then WMI for controllers that do not expose SMART over
// the direct path (commonly some USB, RAID, and policy-restricted devices).
//
// Successfully collected native data always remains authoritative. WMI only
// fills an empty/corrupt ATA result or restores a disk omitted by IOCTL.
func DiscoverWithFallback() ([]Disk, error) {
	primary, err := Discover()
	if err != nil || len(primary) == 0 {
		return DiscoverWMI()
	}

	fallback, wmiErr := DiscoverWMI()
	if wmiErr != nil {
		return primary, nil
	}
	return MergeFallbackDisks(primary, fallback), nil
}

// MergeFallbackDisks combines a native scan and the WMI fallback without
// overwriting valid native readings. It is exported so the GUI and startup
// paths can share the exact same safety rules.
func MergeFallbackDisks(primary, fallback []Disk) []Disk {
	byIndex := make(map[int]Disk, len(fallback))
	for _, d := range fallback {
		byIndex[d.Index] = d
	}
	seen := make(map[int]bool, len(primary))
	result := make([]Disk, 0, len(primary)+len(fallback))
	for _, d := range primary {
		seen[d.Index] = true
		if f, ok := byIndex[d.Index]; ok && f.Kind == d.Kind {
			primaryCorrupt := d.Kind == KindATA && d.SMARTChecksumKnown && !d.SMARTChecksumValid
			fallbackCanReplace := len(d.Attrs) == 0 || (primaryCorrupt && f.SMARTChecksumKnown && f.SMARTChecksumValid)
			if fallbackCanReplace && len(f.Attrs) > 0 {
				d.Attrs = f.Attrs
				d.SMARTTransport = f.SMARTTransport
				if f.SMARTChecksumKnown {
					d.SMARTChecksumKnown = true
					d.SMARTChecksumValid = f.SMARTChecksumValid
				}
			}
			if !d.SmartStatusKnown && f.SmartStatusKnown {
				d.SmartStatusKnown = true
				d.SmartStatusPassed = f.SmartStatusPassed
			}
		}
		result = append(result, d)
	}
	for _, d := range fallback {
		if !seen[d.Index] {
			result = append(result, d)
		}
	}
	return result
}
