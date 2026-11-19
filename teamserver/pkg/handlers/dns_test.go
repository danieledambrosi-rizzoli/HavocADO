package handlers

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestNormalizeDNSDomain(t *testing.T) {
	tests := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"c2.example.com", "c2.example.com", false},
		{"C2.Example.COM.", "c2.example.com", false},
		{"  c2.example.com  ", "c2.example.com", false},
		{"", "", true},
		{"not a domain", "", true},
		{"...", "", true},
	}

	for _, test := range tests {
		got, err := normalizeDNSDomain(test.in)
		if (err != nil) != test.wantErr {
			t.Fatalf("normalizeDNSDomain(%q) error = %v, wantErr %v", test.in, err, test.wantErr)
		}
		if got != test.want {
			t.Fatalf("normalizeDNSDomain(%q) = %q, want %q", test.in, got, test.want)
		}
	}
}

func TestNormalizeDNSRecordTypeAndEncoding(t *testing.T) {
	if got, err := normalizeDNSRecordType(""); err != nil || got != "TXT" {
		t.Fatalf("empty record type = %q, %v", got, err)
	}
	if _, err := normalizeDNSRecordType("A"); err == nil {
		t.Fatal("expected A record type to be rejected")
	}

	if got, err := normalizeDNSEncoding("base64"); err != nil || got != "base64url" {
		t.Fatalf("base64 alias = %q, %v", got, err)
	}
	if _, err := normalizeDNSEncoding("hex"); err == nil {
		t.Fatal("expected hex encoding to be rejected")
	}
}

func TestNormalizeDNSFrequency(t *testing.T) {
	tests := map[int]int{
		0:    DNSDefaultFrequency,
		-10:  DNSDefaultFrequency,
		1:    DNSMinFrequency,
		1000: DNSMaxFrequency,
		45:   45,
	}

	for in, want := range tests {
		if got := normalizeDNSFrequency(in); got != want {
			t.Fatalf("normalizeDNSFrequency(%d) = %d, want %d", in, got, want)
		}
	}
}

func TestParseQueryName(t *testing.T) {
	listener := NewConfigDns()
	listener.Config.Domain = "c2.example.com"

	payload := base64.RawURLEncoding.EncodeToString([]byte("chunk-data"))
	name := "0.1.deadbeef." + payload + ".c2.example.com."

	seq, total, token, data, ok := listener.parseQueryName(name)
	if !ok {
		t.Fatalf("parseQueryName(%q) failed", name)
	}
	if seq != 0 || total != 1 || token != "deadbeef" || data != payload {
		t.Fatalf("parseQueryName(%q) = %d, %d, %q, %q", name, seq, total, token, data)
	}

	// Labels after the token are joined back together.
	split := "0.1.deadbeef." + payload[:4] + "." + payload[4:] + ".c2.example.com."
	if _, _, _, joined, ok := listener.parseQueryName(split); !ok || joined != payload {
		t.Fatalf("parseQueryName did not rejoin labels: %q ok=%v", joined, ok)
	}

	// A domain stored with a trailing dot must match the same names.
	listener.Config.Domain = "c2.example.com."
	if _, _, _, _, ok := listener.parseQueryName(name); !ok {
		t.Fatal("trailing-dot domain did not match")
	}

	// The token is case-insensitive; the data labels must keep their exact
	// case, because base64url is case-sensitive.
	if _, _, token, _, ok := listener.parseQueryName("0.1.DEADBEEF." + payload + ".c2.example.com."); !ok || token != "deadbeef" {
		t.Fatalf("upper-case token = %q ok=%v", token, ok)
	}

	rejected := []string{
		"c2.example.com.",
		"0.1.deadbeef.c2.example.com.",
		"1.1.deadbeef." + payload + ".c2.example.com.",
		"0.1.deadbeef." + payload + ".other.example.com.",
		"x.1.deadbeef." + payload + ".c2.example.com.",
	}

	for _, name := range rejected {
		if _, _, _, _, ok := listener.parseQueryName(name); ok {
			t.Fatalf("parseQueryName(%q) should have failed", name)
		}
	}
}

func TestSplitTXTStrings(t *testing.T) {
	if got := splitTXTStrings(""); got != nil {
		t.Fatalf("empty split = %#v", got)
	}

	value := strings.Repeat("a", DNSTXTStringChars+10)
	parts := splitTXTStrings(value)
	if len(parts) != 2 {
		t.Fatalf("split into %d parts, want 2", len(parts))
	}
	if len(parts[0]) != DNSTXTStringChars || len(parts[1]) != 10 {
		t.Fatalf("split lengths = %d, %d", len(parts[0]), len(parts[1]))
	}
	if strings.Join(parts, "") != value {
		t.Fatal("split dropped data")
	}
}
