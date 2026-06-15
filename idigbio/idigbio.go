// Package idigbio is the library behind the idigbio command line:
// the HTTP client, request shaping, and the typed data models for iDigBio
// (Integrated Digitized Biocollections).
//
// The Client here is the spine every command shares. It sets a real
// User-Agent, paces requests so a busy session stays polite, and retries the
// transient failures (429 and 5xx) that any public API throws under load.
// Search endpoints use POST with a JSON body; the count endpoint uses GET.
package idigbio

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// DefaultUserAgent identifies the client to iDigBio.
const DefaultUserAgent = "idigbio/dev (+https://github.com/tamnd/idigbio-cli)"

// Host is the API host this client talks to.
const Host = "search.idigbio.org"

// BaseURL is the API root every request is built from.
const BaseURL = "https://" + Host + "/v2"

// Client talks to the iDigBio API over HTTP.
type Client struct {
	HTTP      *http.Client
	UserAgent string
	// Rate is the minimum gap between requests. Zero means no pacing.
	Rate    time.Duration
	Retries int

	last time.Time
}

// NewClient returns a Client with sensible defaults: a 30s timeout, a 300ms
// minimum gap between requests, and three retries on transient errors.
func NewClient() *Client {
	return &Client{
		HTTP:      &http.Client{Timeout: 30 * time.Second},
		UserAgent: DefaultUserAgent,
		Rate:      300 * time.Millisecond,
		Retries:   3,
	}
}

// Get fetches url and returns the response body. It paces and retries
// according to the client's settings.
func (c *Client) Get(ctx context.Context, url string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt <= c.Retries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff(attempt)):
			}
		}
		body, retry, err := c.do(ctx, http.MethodGet, url, nil, "")
		if err == nil {
			return body, nil
		}
		lastErr = err
		if !retry {
			return nil, err
		}
	}
	return nil, fmt.Errorf("get %s: %w", url, lastErr)
}

// Post sends a JSON body to url and returns the response body. It paces and
// retries on transient errors.
func (c *Client) Post(ctx context.Context, url string, bodyBytes []byte) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt <= c.Retries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff(attempt)):
			}
		}
		body, retry, err := c.do(ctx, http.MethodPost, url, bodyBytes, "application/json")
		if err == nil {
			return body, nil
		}
		lastErr = err
		if !retry {
			return nil, err
		}
	}
	return nil, fmt.Errorf("post %s: %w", url, lastErr)
}

func (c *Client) do(ctx context.Context, method, url string, bodyBytes []byte, contentType string) (body []byte, retry bool, err error) {
	c.pace()
	var bodyReader io.Reader
	if bodyBytes != nil {
		bodyReader = bytes.NewReader(bodyBytes)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("User-Agent", c.UserAgent)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, true, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return nil, true, fmt.Errorf("http %d", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("http %d", resp.StatusCode)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, true, err
	}
	return b, false, nil
}

// pace blocks until at least Rate has passed since the previous request.
func (c *Client) pace() {
	if c.Rate <= 0 {
		return
	}
	if wait := c.Rate - time.Since(c.last); wait > 0 {
		time.Sleep(wait)
	}
	c.last = time.Now()
}

func backoff(attempt int) time.Duration {
	d := time.Duration(attempt) * 500 * time.Millisecond
	if d > 5*time.Second {
		d = 5 * time.Second
	}
	return d
}

// --- wire types ---

type wireIndexTerms struct {
	UUID            string `json:"uuid"`
	ScientificName  string `json:"scientificname"`
	Family          string `json:"family"`
	Genus           string `json:"genus"`
	Country         string `json:"country"`
	CountryCode     string `json:"countrycode"`
	Continent       string `json:"continent"`
	BasisOfRecord   string `json:"basisofrecord"`
	InstitutionCode string `json:"institutioncode"`
	CatalogNumber   string `json:"catalognumber"`
	Collector       string `json:"collector"`
	DateCollected   string `json:"datecollected"`
	TypeStatus      string `json:"typestatus"`
	HasMedia        bool   `json:"hasMedia"`
}

type wireRecord struct {
	UUID       string         `json:"uuid"`
	IndexTerms wireIndexTerms `json:"indexTerms"`
}

type wireSearchResp struct {
	ItemCount int          `json:"itemCount"`
	Items     []wireRecord `json:"items"`
}

type wireCountResp struct {
	ItemCount int `json:"itemCount"`
}

// --- public types ---

// Record is a single digitized natural history specimen record from iDigBio.
type Record struct {
	ID             string `json:"id"               kit:"id"`
	ScientificName string `json:"scientific_name"`
	Family         string `json:"family,omitempty"`
	Genus          string `json:"genus,omitempty"`
	Country        string `json:"country,omitempty"`
	Continent      string `json:"continent,omitempty"`
	BasisOfRecord  string `json:"basis_of_record,omitempty"`
	Institution    string `json:"institution,omitempty"`
	CatalogNumber  string `json:"catalog_number,omitempty"`
	Collector      string `json:"collector,omitempty"`
	DateCollected  string `json:"date_collected,omitempty"`
	TypeStatus     string `json:"type_status,omitempty"`
	HasMedia       bool   `json:"has_media,omitempty"`
}

// CountResult holds the total specimen record count.
type CountResult struct {
	Total int `json:"total"`
}

// --- client methods ---

// SearchRecords searches for specimen records by scientific name.
// It returns up to limit records starting at offset, and the total count.
func (c *Client) SearchRecords(ctx context.Context, scientificName string, limit, offset int) ([]*Record, int, error) {
	url := fmt.Sprintf("%s/search/records/?limit=%d&offset=%d", BaseURL, limit, offset)
	payload, err := json.Marshal(map[string]any{"rq": map[string]string{"scientificname": scientificName}})
	if err != nil {
		return nil, 0, fmt.Errorf("encode search body: %w", err)
	}

	body, err := c.Post(ctx, url, payload)
	if err != nil {
		return nil, 0, err
	}

	var resp wireSearchResp
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, 0, fmt.Errorf("decode search records: %w", err)
	}

	out := make([]*Record, 0, len(resp.Items))
	for _, w := range resp.Items {
		out = append(out, recordFromWire(w))
	}
	return out, resp.ItemCount, nil
}

// ListRecords lists all specimen records, up to limit starting at offset.
// It returns the records and the total count.
func (c *Client) ListRecords(ctx context.Context, limit, offset int) ([]*Record, int, error) {
	url := fmt.Sprintf("%s/search/records/?limit=%d&offset=%d", BaseURL, limit, offset)
	payload, err := json.Marshal(map[string]any{"rq": map[string]any{}})
	if err != nil {
		return nil, 0, fmt.Errorf("encode list body: %w", err)
	}

	body, err := c.Post(ctx, url, payload)
	if err != nil {
		return nil, 0, err
	}

	var resp wireSearchResp
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, 0, fmt.Errorf("decode list records: %w", err)
	}

	out := make([]*Record, 0, len(resp.Items))
	for _, w := range resp.Items {
		out = append(out, recordFromWire(w))
	}
	return out, resp.ItemCount, nil
}

// GetRecord fetches a single specimen record by its UUID.
func (c *Client) GetRecord(ctx context.Context, uuid string) (*Record, error) {
	url := BaseURL + "/view/records/" + uuid

	body, err := c.Get(ctx, url)
	if err != nil {
		return nil, err
	}

	var resp wireRecord
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode record: %w", err)
	}
	if resp.UUID == "" {
		resp.UUID = uuid
	}

	return recordFromWire(resp), nil
}

// Count returns the total number of digitized specimen records in iDigBio.
func (c *Client) Count(ctx context.Context) (int, error) {
	url := BaseURL + "/summary/count/records/"

	body, err := c.Get(ctx, url)
	if err != nil {
		return 0, err
	}

	var resp wireCountResp
	if err := json.Unmarshal(body, &resp); err != nil {
		return 0, fmt.Errorf("decode count: %w", err)
	}

	return resp.ItemCount, nil
}

// --- conversion helpers ---

func recordFromWire(w wireRecord) *Record {
	it := w.IndexTerms
	return &Record{
		ID:             w.UUID,
		ScientificName: it.ScientificName,
		Family:         it.Family,
		Genus:          it.Genus,
		Country:        it.Country,
		Continent:      it.Continent,
		BasisOfRecord:  it.BasisOfRecord,
		Institution:    it.InstitutionCode,
		CatalogNumber:  it.CatalogNumber,
		Collector:      it.Collector,
		DateCollected:  it.DateCollected,
		TypeStatus:     it.TypeStatus,
		HasMedia:       it.HasMedia,
	}
}
