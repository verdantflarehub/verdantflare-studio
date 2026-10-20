package project

import (
	"context"
	"errors"
	"testing"
)

func TestDraftMetadataDoesNotGrantProjectAuthority(t *testing.T) {
	const id = "01900000-0000-7000-8000-000000000001"
	p := Principal{OrganizationID: id, SubjectID: id, RequestID: id}
	source := Source{Kind: "user_import", WorldDraftID: id, ProjectID: id}
	version := ContentVersion{ContentRef: ContentRef{id, id, id}, OrganizationID: id, Source: source}
	// A nil database also verifies rejection occurs before any project lookup.
	s := &Service{}
	for _, permission := range []Permission{
		{Action: "write", Source: &source},
		{Action: "read", Version: &version, Access: &Access{}},
		{Action: "delete", Version: &version, Access: &Access{}},
		{Action: "read", Version: &version, Access: &Access{WorldDraftID: id, WorldDraftRevisionID: id}},
		{Action: "read", Version: &version, Access: &Access{WorldDraftID: id, WorldDraftRevisionID: id, ProjectID: id, RevisionID: id}},
	} {
		if err := s.AuthorizeArtifact(context.Background(), p, permission); !errors.Is(err, ErrForbidden) {
			t.Fatalf("draft scope reached project authority: %v", err)
		}
	}
}
