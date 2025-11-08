package main

import (
	// "log"
	"net/netip"
	"slices"
	"testing"
)


func TestSprintTime(t *testing.T) {

	ll := netip.MustParseAddr("fe80::53eb:d72c:9d70:5b33")
	ula := netip.MustParseAddr("fd0e::cafe:babe:beef")
	gua := netip.MustParseAddr("3fff::cafe:babe:beef")

	rfc1918 := netip.MustParseAddr("10.0.52.10")

	t.Run("ipv6 test", func(t *testing.T) {
		t.Parallel()

		if isIPv6(&ll) {
			t.Fail()
		}
	})

	t.Run("ipv6 test", func(t *testing.T) {
		t.Parallel()

		if !isIPv6(&ula) {
			t.Fail()
		}
	})

	t.Run("ipv6 test", func(t *testing.T) {
		t.Parallel()

		if !isIPv6(&gua) {
			t.Fail()
		}
	})

	t.Run("ipv4 test", func(t *testing.T) {
		t.Parallel()

		if isIPv4(&gua) {
			t.Fail()
		}
	})

	t.Run("ipv4 test", func(t *testing.T) {
		t.Parallel()

		if !isIPv4(&rfc1918) {
			t.Fail()
		}
	})

	t.Run("hostname info", func(t *testing.T) {
		t.Parallel()
		hn := "test.1.2.3.4"
		hns := getHostnameSplits(hn)
		if !slices.Equal(hns, []string{"test", "test.1", "test.1.2", "test.1.2.3"}) {
			t.Fatal(hns)

		}

	})




}
