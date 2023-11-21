package main

import "testing"

// tests merge both full
func TestMerge(t *testing.T) {
	c := EmptyMasqConfig()

	c1 := &MasqConfig{
		NonMasqueradeCIDRs: []string{"155.128.0.0/9", "10.240.0.0/16", "180.132.128.0/18", "3.3.3.0/24"},
		MasqLinkLocal:      false,
		MasqLinkLocalIPv6:  true,
	}

	want := &MasqConfig{
		NonMasqueradeCIDRs: []string{"155.128.0.0/9", "10.240.0.0/16", "180.132.128.0/18", "3.3.3.0/24"},
		MasqLinkLocal:      false,
		MasqLinkLocalIPv6:  true,
	}

	c.merge(c1)
	if !c.equals(want) {
		t.Errorf("c.merge(c1) wrote %v, want %v", c, want)
	}

	c2 := &MasqConfig{
		NonMasqueradeCIDRs: []string{"1.0.0.0/8", "2.2.0.0/16", "3.3.3.0/24"},
		MasqLinkLocal:      true,
		MasqLinkLocalIPv6:  false,
	}

	want = &MasqConfig{
		NonMasqueradeCIDRs: []string{"155.128.0.0/9", "10.240.0.0/16", "180.132.128.0/18", "3.3.3.0/24", "1.0.0.0/8", "2.2.0.0/16"},
		MasqLinkLocal:      true,
		MasqLinkLocalIPv6:  true,
	}

	c.merge(c2)
	if !c.equals(want) {
		t.Errorf("c.merge(c2) wrote %v, want %v", c, want)
	}
}

// tests merge handling missing fields
func TestMergeIncomplete(t *testing.T) {
	c := EmptyMasqConfig()

	c1 := &MasqConfig{
		NonMasqueradeCIDRs: []string{"10.128.0.192/26", "100.16.0.0/12"},
	}

	c.merge(c1)

	want := &MasqConfig{
		NonMasqueradeCIDRs: []string{"10.128.0.192/26", "100.16.0.0/12"},
		MasqLinkLocal:      false,
		MasqLinkLocalIPv6:  false,
	}

	c.merge(c1)
	if !c.equals(want) {
		t.Errorf("c.merge(c1) [missing fields] wrote %v, want %v", c, want)
	}

	c2 := &MasqConfig{
		MasqLinkLocal: true,
	}

	want = &MasqConfig{
		NonMasqueradeCIDRs: []string{"10.128.0.192/26", "100.16.0.0/12"},
		MasqLinkLocal:      true,
		MasqLinkLocalIPv6:  false,
	}

	c.merge(c2)
	if !c.equals(want) {
		t.Errorf("c.merge(c2) [missing fields] wrote %v, want %v", c, want)
	}
}

func (c *MasqConfig) equals(newConfig *MasqConfig) bool {
	return slicesEqual(c.NonMasqueradeCIDRs, newConfig.NonMasqueradeCIDRs) &&
		c.MasqLinkLocal == newConfig.MasqLinkLocal &&
		c.MasqLinkLocalIPv6 == newConfig.MasqLinkLocalIPv6
}

// Ignore ordering, just check if size and elements are the same
func slicesEqual(x, y []string) bool {
	if len(x) != len(y) {
		return false
	}

	xMap := make(map[string]int)
	yMap := make(map[string]int)

	for _, xElem := range x {
		xMap[xElem]++
	}
	for _, yElem := range y {
		yMap[yElem]++
	}

	for xMapKey, xMapVal := range xMap {
		if yMap[xMapKey] != xMapVal {
			return false
		}
	}
	return true
}
