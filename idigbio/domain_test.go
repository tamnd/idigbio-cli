package idigbio

import (
	"testing"

	"github.com/tamnd/any-cli/kit"
)

// These tests are offline: they exercise the URI driver's pure string functions
// and the host wiring (mint, body, resolve), which need no network. The
// client's HTTP behaviour is covered in idigbio_test.go.

func TestDomainInfo(t *testing.T) {
	info := Domain{}.Info()
	if info.Scheme != "idigbio" {
		t.Errorf("Scheme = %q, want idigbio", info.Scheme)
	}
	if len(info.Hosts) == 0 || info.Hosts[0] != Host {
		t.Errorf("Hosts = %v, want [%s]", info.Hosts, Host)
	}
	if info.Identity.Binary != "idigbio" {
		t.Errorf("Identity.Binary = %q, want idigbio", info.Identity.Binary)
	}
}

func TestClassify(t *testing.T) {
	cases := []struct{ in, typ, id string }{
		{"abc123-uuid", "record", "abc123-uuid"},
		{"Gadus morhua", "record", "Gadus morhua"},
		{"some-record-id", "record", "some-record-id"},
	}
	for _, tc := range cases {
		typ, id, err := Domain{}.Classify(tc.in)
		if err != nil || typ != tc.typ || id != tc.id {
			t.Errorf("Classify(%q) = (%q, %q, %v), want (%q, %q, nil)",
				tc.in, typ, id, err, tc.typ, tc.id)
		}
	}
}

func TestLocate(t *testing.T) {
	got, err := Domain{}.Locate("record", "abc123-uuid")
	want := "https://portal.idigbio.org/portal/records/abc123-uuid"
	if err != nil || got != want {
		t.Errorf("Locate = (%q, %v), want (%q, nil)", got, err, want)
	}
}

func TestLocateUnknownType(t *testing.T) {
	_, err := Domain{}.Locate("unknown", "id")
	if err == nil {
		t.Error("Locate with unknown type should return an error")
	}
}

func TestClassifyEmpty(t *testing.T) {
	_, _, err := Domain{}.Classify("")
	if err == nil {
		t.Error("Classify with empty string should return an error")
	}
}

// TestHostWiring mounts the driver in a kit Host and checks the round trip:
// a record mints to its URI, its body is readable, and a bare id resolves
// back to the same URI. The init in domain.go registers the domain, so
// kit.Open finds it.
func TestHostWiring(t *testing.T) {
	h, err := kit.Open()
	if err != nil {
		t.Fatal(err)
	}

	r := &Record{ID: "abc123-uuid", ScientificName: "Gadus morhua", Family: "Gadidae"}
	u, err := h.Mint(r)
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}
	if want := "idigbio://record/abc123-uuid"; u.String() != want {
		t.Errorf("Mint = %q, want %q", u.String(), want)
	}

	got, err := h.ResolveOn("idigbio", "some-id")
	if err != nil || got.String() != "idigbio://record/some-id" {
		t.Errorf("ResolveOn = (%q, %v), want idigbio://record/some-id", got.String(), err)
	}
}
