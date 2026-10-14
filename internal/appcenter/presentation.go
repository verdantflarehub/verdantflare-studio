package appcenter

import (
	"bytes"
	"context"
	"io"
	"io/fs"
	"os"
	"strings"
	"time"
)

type AppOrdering struct {
	PriorityClass string `json:"priorityClass"`
	Order         *int   `json:"order,omitempty"`
}

type PresentationRecord struct {
	AppID        string       `json:"appId"`
	AppVersion   *string      `json:"appVersion"`
	Display      Display      `json:"display"`
	Ordering     *AppOrdering `json:"ordering,omitempty"`
	SourceRef    string       `json:"sourceRef"`
	SourceDigest string       `json:"sourceDigest"`
}
type PresentationRegistry struct {
	SchemaVersion int                  `json:"schemaVersion"`
	Revision      string               `json:"revision"`
	Groups        []Group              `json:"groups"`
	Records       []PresentationRecord `json:"records"`
}

func ReadPresentation(r io.Reader) (PresentationRegistry, error) {
	var reg PresentationRegistry
	if Decode(r, "presentation", 1<<20, &reg) != nil {
		return reg, ErrInvalid
	}
	groups := map[string]bool{}
	for _, g := range reg.Groups {
		if groups[g.ID] {
			return reg, ErrInvalid
		}
		groups[g.ID] = true
	}
	records := map[string]bool{}
	for _, record := range reg.Records {
		version := "*"
		if record.AppVersion != nil {
			version = *record.AppVersion
		}
		key := record.AppID + "@" + version
		if records[key] || !groups[record.Display.GroupID] || record.Display.Quality != "known" || !fs.ValidPath(record.SourceRef) || strings.ContainsAny(record.SourceRef, "\\:") || record.SourceRef == "." {
			return reg, ErrInvalid
		}
		records[key] = true
	}
	return reg, nil
}

// Presentation metadata never creates an entry or permission. Missing optional
// registration is visible partial data; a configured malformed file is an error.
func LoadPresentation(ctx context.Context, path string, facts Facts) ([]Group, map[string]Metadata, Source, error) {
	source := Source{Source: "presentation", Quality: "unknown", ReasonCode: Text("PRESENTATION_UNREGISTERED")}
	metadata := map[string]Metadata{}
	if err := ctx.Err(); err != nil {
		return nil, nil, source, err
	}
	if path == "" {
		return []Group{}, metadata, source, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, source, ErrUnavailable
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
		return nil, nil, source, ErrInvalid
	}
	raw, err := io.ReadAll(io.LimitReader(f, (1<<20)+1))
	if err != nil || len(raw) > 1<<20 {
		return nil, nil, source, ErrInvalid
	}
	reg, err := ReadPresentation(bytes.NewReader(raw))
	if err != nil {
		return nil, nil, source, err
	}
	records := map[string]Display{}
	ordering := map[string]*AppOrdering{}
	for _, r := range reg.Records {
		version := "*"
		if r.AppVersion != nil {
			version = *r.AppVersion
		} else {
			ordering[r.AppID] = r.Ordering
		}
		records[r.AppID+"@"+version] = r.Display
	}
	for _, fact := range facts.Items {
		_, candidate := catalogDisplay(fact)
		version := ""
		if candidate != nil {
			version = *candidate
		}
		if fact.Installation != nil {
			version = fact.Installation.AppVersion
		}
		display, ok := records[fact.AppID+"@"+version]
		if !ok {
			display, ok = records[fact.AppID+"@*"]
		}
		if ok {
			copy := display
			metadata[fact.CardKey] = Metadata{Display: &copy, Ordering: ordering[fact.AppID]}
		}
	}
	now := time.Now().UTC()
	source = Source{Source: "presentation", Quality: "known", Revision: Text(Digest(reg)), ObservedAt: &now}
	return reg.Groups, metadata, source, nil
}
