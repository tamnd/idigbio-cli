package idigbio

import (
	"context"

	"github.com/tamnd/any-cli/kit"
	"github.com/tamnd/any-cli/kit/errs"
)

// domain.go exposes iDigBio as a kit Domain: a driver that a multi-domain
// host (ant) enables with a single blank import,
//
//	import _ "github.com/tamnd/idigbio-cli/idigbio"
//
// exactly as a database/sql program enables a driver with `import _
// "github.com/lib/pq"`. The init below registers it; the host then
// dereferences idigbio:// URIs by routing to the operations Register installs.
// The same Domain also builds the standalone idigbio binary (see cli.NewApp),
// so the binary and a host share one source of truth.
func init() { kit.Register(Domain{}) }

// Domain is the iDigBio driver. It carries no state; the per-run client is
// built by the factory Register hands kit.
type Domain struct{}

// Info describes the scheme, the hostnames a pasted link is matched against,
// and the identity reused for the binary's help and version.
func (Domain) Info() kit.DomainInfo {
	return kit.DomainInfo{
		Scheme: "idigbio",
		Hosts:  []string{Host},
		Identity: kit.Identity{
			Binary: "idigbio",
			Short:  "iDigBio specimen records CLI",
			Long: `idigbio reads public iDigBio data over plain HTTPS, shapes it into
clean records, and prints output that pipes into the rest of your tools.
No API key, nothing to run alongside it.

iDigBio (Integrated Digitized Biocollections) aggregates 150M+ digitized
natural history specimen records from institutions worldwide.`,
			Site: Host,
			Repo: "https://github.com/tamnd/idigbio-cli",
		},
	}
}

// Register installs the client factory and every iDigBio operation onto app.
func (Domain) Register(app *kit.App) {
	app.SetClient(newClient)

	kit.Handle(app, kit.OpMeta{Name: "get", Group: "read", Single: true,
		Summary: "Fetch a specimen record by UUID", URIType: "record", Resolver: true,
		Args: []kit.Arg{{Name: "uuid", Help: "specimen record UUID"}}},
		getRecord)

	kit.Handle(app, kit.OpMeta{Name: "search", Group: "read", List: true,
		Summary: "Search specimen records by scientific name",
		Args:    []kit.Arg{{Name: "scientific-name", Help: "scientific name to search for"}}},
		searchRecords)

	kit.Handle(app, kit.OpMeta{Name: "records", Group: "read", List: true,
		Summary: "List all specimen records"},
		listRecords)

	kit.Handle(app, kit.OpMeta{Name: "count", Group: "read", Single: true,
		Summary: "Show total specimen record count"},
		countRecords)
}

// newClient builds the iDigBio client from the host-resolved config.
func newClient(_ context.Context, cfg kit.Config) (any, error) {
	c := NewClient()
	if cfg.UserAgent != "" {
		c.UserAgent = cfg.UserAgent
	}
	if cfg.Rate > 0 {
		c.Rate = cfg.Rate
	}
	if cfg.Retries > 0 {
		c.Retries = cfg.Retries
	}
	if cfg.Timeout > 0 {
		c.HTTP.Timeout = cfg.Timeout
	}
	return c, nil
}

// --- inputs ---

type getIn struct {
	UUID   string  `kit:"arg"    help:"specimen record UUID"`
	Client *Client `kit:"inject"`
}

type searchIn struct {
	Name   string  `kit:"arg"          help:"scientific name to search for"`
	Limit  int     `kit:"flag,inherit" help:"max results"`
	Offset int     `kit:"flag,inherit" help:"result offset"`
	Client *Client `kit:"inject"`
}

type listIn struct {
	Limit  int     `kit:"flag,inherit" help:"max results"`
	Offset int     `kit:"flag,inherit" help:"result offset"`
	Client *Client `kit:"inject"`
}

type countIn struct {
	Client *Client `kit:"inject"`
}

// --- handlers ---

func getRecord(ctx context.Context, in getIn, emit func(*Record) error) error {
	r, err := in.Client.GetRecord(ctx, in.UUID)
	if err != nil {
		return err
	}
	return emit(r)
}

func searchRecords(ctx context.Context, in searchIn, emit func(*Record) error) error {
	recs, _, err := in.Client.SearchRecords(ctx, in.Name, in.Limit, in.Offset)
	if err != nil {
		return err
	}
	for _, r := range recs {
		if err := emit(r); err != nil {
			return err
		}
	}
	return nil
}

func listRecords(ctx context.Context, in listIn, emit func(*Record) error) error {
	recs, _, err := in.Client.ListRecords(ctx, in.Limit, in.Offset)
	if err != nil {
		return err
	}
	for _, r := range recs {
		if err := emit(r); err != nil {
			return err
		}
	}
	return nil
}

func countRecords(ctx context.Context, in countIn, emit func(*CountResult) error) error {
	n, err := in.Client.Count(ctx)
	if err != nil {
		return err
	}
	return emit(&CountResult{Total: n})
}

// --- Resolver: URI-native string functions ---

// Classify turns any accepted input into (type, id). Any non-empty string
// maps to ("record", input) so it is addressable as a resource URI.
func (Domain) Classify(input string) (uriType, id string, err error) {
	if input == "" {
		return "", "", errs.Usage("idigbio: empty reference")
	}
	return "record", input, nil
}

// Locate returns the live portal URL for a (type, id).
func (Domain) Locate(uriType, id string) (string, error) {
	switch uriType {
	case "record":
		return "https://portal.idigbio.org/portal/records/" + id, nil
	default:
		return "", errs.Usage("idigbio has no resource type %q", uriType)
	}
}
