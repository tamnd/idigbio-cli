package idigbio

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestGet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			t.Error("request carried no User-Agent")
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	c := NewClient()
	c.Rate = 0 // no pacing in the test

	body, err := c.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "ok" {
		t.Errorf("body = %q, want %q", body, "ok")
	}
}

func TestGetRetriesOn503(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if hits < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("recovered"))
	}))
	defer srv.Close()

	c := NewClient()
	c.Rate = 0
	c.Retries = 5

	start := time.Now()
	body, err := c.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "recovered" {
		t.Errorf("body = %q after retries", body)
	}
	if hits != 3 {
		t.Errorf("server saw %d hits, want 3", hits)
	}
	if time.Since(start) < 500*time.Millisecond {
		t.Error("retries did not back off")
	}
}

func TestSearchRecords(t *testing.T) {
	payload := wireSearchResp{
		ItemCount: 1168,
		Items: []wireRecord{
			{
				UUID: "uuid-gadus-1",
				IndexTerms: wireIndexTerms{
					UUID:            "uuid-gadus-1",
					ScientificName:  "Gadus morhua",
					Family:          "Gadidae",
					Genus:           "Gadus",
					Country:         "Canada",
					Continent:       "North America",
					BasisOfRecord:   "PreservedSpecimen",
					InstitutionCode: "MCZ",
					CatalogNumber:   "MCZ:Fish:12345",
					Collector:       "Smith, J.",
					DateCollected:   "1985-06-15",
					TypeStatus:      "",
					HasMedia:        false,
				},
			},
			{
				UUID: "uuid-gadus-2",
				IndexTerms: wireIndexTerms{
					UUID:           "uuid-gadus-2",
					ScientificName: "Gadus morhua",
					Family:         "Gadidae",
					Country:        "Norway",
					BasisOfRecord:  "PreservedSpecimen",
					HasMedia:       true,
				},
			},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %q, want POST", r.Method)
		}
		if r.URL.Path != "/v2/search/records/" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", ct)
		}
		var reqBody map[string]any
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Errorf("decode request body: %v", err)
		}
		rq, _ := reqBody["rq"].(map[string]any)
		if rq["scientificname"] != "Gadus morhua" {
			t.Errorf("rq.scientificname = %v, want Gadus morhua", rq["scientificname"])
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(payload)
	}))
	defer srv.Close()

	c := NewClient()
	c.Rate = 0
	c.HTTP.Transport = &prefixTransport{prefix: srv.URL, inner: http.DefaultTransport}

	recs, total, err := c.SearchRecords(context.Background(), "Gadus morhua", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if total != 1168 {
		t.Errorf("total = %d, want 1168", total)
	}
	if len(recs) != 2 {
		t.Fatalf("len(recs) = %d, want 2", len(recs))
	}
	if recs[0].ID != "uuid-gadus-1" {
		t.Errorf("recs[0].ID = %q, want uuid-gadus-1", recs[0].ID)
	}
	if recs[0].ScientificName != "Gadus morhua" {
		t.Errorf("ScientificName = %q", recs[0].ScientificName)
	}
	if recs[0].Family != "Gadidae" {
		t.Errorf("Family = %q, want Gadidae", recs[0].Family)
	}
	if recs[0].Country != "Canada" {
		t.Errorf("Country = %q, want Canada", recs[0].Country)
	}
	if recs[0].Institution != "MCZ" {
		t.Errorf("Institution = %q, want MCZ", recs[0].Institution)
	}
	if recs[1].HasMedia != true {
		t.Errorf("recs[1].HasMedia = false, want true")
	}
}

func TestListRecords(t *testing.T) {
	payload := wireSearchResp{
		ItemCount: 150902163,
		Items: []wireRecord{
			{
				UUID: "uuid-list-1",
				IndexTerms: wireIndexTerms{
					UUID:           "uuid-list-1",
					ScientificName: "Quercus robur",
					Family:         "Fagaceae",
					Country:        "United Kingdom",
					BasisOfRecord:  "PreservedSpecimen",
				},
			},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %q, want POST", r.Method)
		}
		if r.URL.Path != "/v2/search/records/" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		var reqBody map[string]any
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Errorf("decode request body: %v", err)
		}
		rq, _ := reqBody["rq"].(map[string]any)
		if len(rq) != 0 {
			t.Errorf("rq should be empty for ListRecords, got %v", rq)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(payload)
	}))
	defer srv.Close()

	c := NewClient()
	c.Rate = 0
	c.HTTP.Transport = &prefixTransport{prefix: srv.URL, inner: http.DefaultTransport}

	recs, total, err := c.ListRecords(context.Background(), 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	if total != 150902163 {
		t.Errorf("total = %d, want 150902163", total)
	}
	if len(recs) != 1 {
		t.Fatalf("len(recs) = %d, want 1", len(recs))
	}
	if recs[0].ID != "uuid-list-1" {
		t.Errorf("recs[0].ID = %q, want uuid-list-1", recs[0].ID)
	}
	if recs[0].ScientificName != "Quercus robur" {
		t.Errorf("ScientificName = %q, want Quercus robur", recs[0].ScientificName)
	}
}

func TestCount(t *testing.T) {
	payload := wireCountResp{ItemCount: 150902163}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %q, want GET", r.Method)
		}
		if r.URL.Path != "/v2/summary/count/records/" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(payload)
	}))
	defer srv.Close()

	c := NewClient()
	c.Rate = 0
	c.HTTP.Transport = &prefixTransport{prefix: srv.URL, inner: http.DefaultTransport}

	count, err := c.Count(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if count != 150902163 {
		t.Errorf("count = %d, want 150902163", count)
	}
}

// prefixTransport rewrites request URLs so tests point to the httptest server
// instead of the real iDigBio API, while preserving the path/query.
type prefixTransport struct {
	prefix string
	inner  http.RoundTripper
}

func (t *prefixTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r2 := r.Clone(r.Context())
	u := *r.URL
	u.Scheme = "http"
	host := t.prefix
	if len(host) > 7 && host[:7] == "http://" {
		host = host[7:]
	}
	u.Host = host
	r2.URL = &u
	return t.inner.RoundTrip(r2)
}
