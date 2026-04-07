package providers

import "testing"

func TestNewFallsBackToGenericForUnknownHTTPSHost(t *testing.T) {
	p, err := New("https://downloads.example.test/tool", "")
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if p.GetID() != "generic" {
		t.Fatalf("expected generic provider, got %q", p.GetID())
	}
}

func TestNewForcedGenericProvider(t *testing.T) {
	p, err := New("downloads.example.test/tool", "generic")
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if p.GetID() != "generic" {
		t.Fatalf("expected generic provider, got %q", p.GetID())
	}
}
