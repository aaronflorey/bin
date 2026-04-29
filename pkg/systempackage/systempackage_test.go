package systempackage

import "testing"

func TestDetectType(t *testing.T) {
	tests := []struct {
		name string
		want string
		ok   bool
	}{
		{name: "tool.flatpak", want: "flatpak", ok: true},
		{name: "tool.flatpack", want: "flatpak", ok: true},
		{name: "tool.deb", want: "deb", ok: true},
		{name: "tool.rpm", want: "rpm", ok: true},
		{name: "tool.apk", want: "apk", ok: true},
		{name: "tool.dmg", want: "dmg", ok: true},
		{name: "tool.tar.gz", want: "", ok: false},
	}

	for _, tt := range tests {
		got, ok := DetectType(tt.name)
		if got != tt.want || ok != tt.ok {
			t.Fatalf("DetectType(%q) = (%q, %v), want (%q, %v)", tt.name, got, ok, tt.want, tt.ok)
		}
	}
}

func TestNormalizeType(t *testing.T) {
	if got := NormalizeType(" flatpack "); got != "flatpak" {
		t.Fatalf("unexpected normalized type: %q", got)
	}
	if got := NormalizeType(" DMG "); got != "dmg" {
		t.Fatalf("unexpected normalized type: %q", got)
	}
}

func TestIsKnownType(t *testing.T) {
	if !IsKnownType(" DMG ") {
		t.Fatal("expected dmg to be recognized")
	}
	if IsKnownType("msi") {
		t.Fatal("did not expect msi to be recognized")
	}
}
