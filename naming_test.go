package main

import "testing"

func TestGoIdentifier(t *testing.T) {
	cases := []struct{ in, want string }{
		{"bundle_id", "BundleID"},
		{"active_window_id", "ActiveWindowID"},
		{"source_url", "SourceURL"},
		{"html", "HTML"},
		{"direction", "Direction"},
		{"move_to", "MoveTo"},
		{"snap", "Snap"},
		{"tile_all", "TileAll"},
		{"wm.snap", "WmSnap"},
		{"some-key", "SomeKey"},
	}
	for _, c := range cases {
		got := GoIdentifier(c.in)
		if got != c.want {
			t.Errorf("GoIdentifier(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestCapitalizeFirst(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", ""},
		{"a", "A"},
		{"hello", "Hello"},
		{"HELLO", "Hello"},
	}
	for _, c := range cases {
		got := CapitalizeFirst(c.in)
		if got != c.want {
			t.Errorf("CapitalizeFirst(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
