package project

import (
	"context"
	"encoding/json"
	"errors"
	"mime"
	"sort"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/verdantflarehub/verdantflare-studio/migrations"
)

type Service struct {
	db      *pgxpool.Pool
	content Artifact
	assets  AssetResolver
	workers chan struct{}
}

func NewService(ctx context.Context, db *pgxpool.Pool, content Artifact, assets AssetResolver) (*Service, error) {
	if db == nil || content == nil {
		return nil, ErrInvalid
	}
	if err := migrations.Check(ctx, db); err != nil {
		return nil, err
	}
	s := &Service{db: db, content: content, assets: assets, workers: make(chan struct{}, 8)}
	if s.assets == nil {
		s.assets = s
	}
	return s, nil
}

type queryer interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func role(ctx context.Context, q queryer, p Principal, projectID string) (string, error) {
	if !p.Valid() || !ValidID(projectID) {
		return "", ErrForbidden
	}
	var value string
	err := q.QueryRow(ctx, "SELECT role FROM studio.project_members WHERE organization_id=$1 AND project_id=$2 AND subject_id=$3", p.OrganizationID, projectID, p.SubjectID).Scan(&value)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrForbidden
	}
	return value, err
}
func writable(ctx context.Context, q queryer, p Principal, id string) error {
	v, e := role(ctx, q, p, id)
	if e != nil {
		return e
	}
	if v != "owner" && v != "editor" {
		return ErrForbidden
	}
	return nil
}
func advisory(ctx context.Context, tx pgx.Tx, key string) error {
	_, e := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", key)
	return e
}
func audit(ctx context.Context, tx pgx.Tx, p Principal, action, id string) error {
	_, e := tx.Exec(ctx, "INSERT INTO studio.studio_events(event_id,organization_id,subject_id,request_id,action,resource_id) VALUES($1,$2,$3,$4,$5,$6)", newID(), p.OrganizationID, p.SubjectID, p.RequestID, action, id)
	return e
}

type commitRecord struct {
	ProjectID, CommitID, Actor, Hash, Expected, Revision, State, Error string
	Plan                                                               preparePlan
	Result                                                             *Result
}

func loadCommit(ctx context.Context, q queryer, org, id, commit string) (commitRecord, error) {
	c := commitRecord{ProjectID: id, CommitID: commit}
	var plan, result []byte
	e := q.QueryRow(ctx, `SELECT created_by::text,request_sha256,COALESCE(expected_revision_id::text,''),revision_id::text,state,plan,result,COALESCE(error_code,'') FROM studio.project_commits WHERE organization_id=$1 AND project_id=$2 AND commit_id=$3`, org, id, commit).Scan(&c.Actor, &c.Hash, &c.Expected, &c.Revision, &c.State, &plan, &result, &c.Error)
	if errors.Is(e, pgx.ErrNoRows) {
		return c, ErrNotFound
	}
	if e != nil {
		return c, e
	}
	if e = json.Unmarshal(plan, &c.Plan); e != nil {
		return c, e
	}
	if result != nil {
		e = json.Unmarshal(result, &c.Result)
	}
	return c, e
}
func insertCommit(ctx context.Context, tx pgx.Tx, p Principal, c commitRecord) error {
	plan, e := json.Marshal(c.Plan)
	if e != nil {
		return e
	}
	_, e = tx.Exec(ctx, `INSERT INTO studio.project_commits(organization_id,project_id,commit_id,created_by,request_sha256,expected_revision_id,revision_id,plan) VALUES($1,$2,$3,$4,$5,NULLIF($6,'')::uuid,$7,$8)`, p.OrganizationID, c.ProjectID, c.CommitID, p.SubjectID, c.Hash, c.Expected, c.Revision, plan)
	return e
}

func (s *Service) Create(ctx context.Context, p Principal, r CreateRequest) (Result, error) {
	if !p.Valid() {
		return Result{}, ErrForbidden
	}
	if !ValidID(r.CommitID) {
		return Result{}, ErrInvalid
	}
	hash, e := digest(r)
	if e != nil {
		return Result{}, ErrInvalid
	}
	tx, e := s.db.Begin(ctx)
	if e != nil {
		return Result{}, e
	}
	defer tx.Rollback(context.Background())
	if e = advisory(ctx, tx, "create:"+p.OrganizationID+":"+p.SubjectID+":"+r.CommitID); e != nil {
		return Result{}, e
	}
	var id, prior string
	e = tx.QueryRow(ctx, "SELECT project_id::text,request_sha256 FROM studio.project_create_requests WHERE organization_id=$1 AND subject_id=$2 AND commit_id=$3", p.OrganizationID, p.SubjectID, r.CommitID).Scan(&id, &prior)
	if e == nil {
		if prior != hash {
			return Result{}, ErrIdempotency
		}
	} else if errors.Is(e, pgx.ErrNoRows) {
		id = newID()
		plan, err := initial(r, id)
		if err != nil {
			return Result{}, err
		}
		if _, e = tx.Exec(ctx, "INSERT INTO studio.projects(project_id,organization_id,owner_id,name,category) VALUES($1,$2,$3,$4,$5)", id, p.OrganizationID, p.SubjectID, r.Name, r.Category); e != nil {
			return Result{}, e
		}
		if _, e = tx.Exec(ctx, "INSERT INTO studio.project_members(organization_id,project_id,subject_id,role) VALUES($1,$2,$3,'owner')", p.OrganizationID, id, p.SubjectID); e != nil {
			return Result{}, e
		}
		if _, e = tx.Exec(ctx, "INSERT INTO studio.project_create_requests(organization_id,subject_id,commit_id,project_id,request_sha256) VALUES($1,$2,$3,$4,$5)", p.OrganizationID, p.SubjectID, r.CommitID, id, hash); e != nil {
			return Result{}, e
		}
		if e = insertCommit(ctx, tx, p, commitRecord{ProjectID: id, CommitID: r.CommitID, Hash: hash, Revision: newID(), Plan: plan}); e != nil {
			return Result{}, e
		}
		if e = audit(ctx, tx, p, "project.prepare", id); e != nil {
			return Result{}, e
		}
	} else {
		return Result{}, e
	}
	if e = tx.Commit(ctx); e != nil {
		return Result{}, e
	}
	return s.resume(ctx, p, id, r.CommitID)
}
func (s *Service) Commit(ctx context.Context, p Principal, r CommitRequest) (Result, error) {
	if !ValidID(r.ProjectID) || !ValidID(r.ExpectedRevisionID) || !ValidID(r.CommitID) {
		return Result{}, ErrInvalid
	}
	if e := writable(ctx, s.db, p, r.ProjectID); e != nil {
		return Result{}, e
	}
	data, e := json.Marshal(r)
	if e != nil || len(data) > MaxManifestBytes {
		return Result{}, ErrInvalid
	}
	hash, e := digest(r)
	if e != nil {
		return Result{}, ErrInvalid
	}
	tx, e := s.db.Begin(ctx)
	if e != nil {
		return Result{}, e
	}
	defer tx.Rollback(context.Background())
	if e = advisory(ctx, tx, "commit:"+p.OrganizationID+":"+r.ProjectID+":"+r.CommitID); e != nil {
		return Result{}, e
	}
	c, e := loadCommit(ctx, tx, p.OrganizationID, r.ProjectID, r.CommitID)
	if e == nil {
		if c.Actor != p.SubjectID {
			return Result{}, ErrForbidden
		}
		if c.Hash != hash {
			return Result{}, ErrIdempotency
		}
	} else if errors.Is(e, ErrNotFound) {
		previous, _, err := revision(ctx, tx, p.OrganizationID, r.ProjectID, r.ExpectedRevisionID)
		if err != nil {
			return Result{}, err
		}
		plan, err := changed(previous, r)
		if err != nil {
			return Result{}, err
		}
		c = commitRecord{ProjectID: r.ProjectID, CommitID: r.CommitID, Hash: hash, Expected: r.ExpectedRevisionID, Revision: newID(), Plan: plan}
		if e = insertCommit(ctx, tx, p, c); e != nil {
			return Result{}, e
		}
	} else {
		return Result{}, e
	}
	if e = tx.Commit(ctx); e != nil {
		return Result{}, e
	}
	return s.resume(ctx, p, r.ProjectID, r.CommitID)
}
func revision(ctx context.Context, q queryer, org, project, id string) (Manifest, ContentRef, error) {
	var m Manifest
	var ref ContentRef
	var a, b []byte
	e := q.QueryRow(ctx, "SELECT manifest_json,manifest_ref FROM studio.project_revisions WHERE organization_id=$1 AND project_id=$2 AND revision_id=$3", org, project, id).Scan(&a, &b)
	if errors.Is(e, pgx.ErrNoRows) {
		return m, ref, ErrNotFound
	}
	if e != nil {
		return m, ref, e
	}
	if e = json.Unmarshal(a, &m); e != nil {
		return m, ref, e
	}
	e = json.Unmarshal(b, &ref)
	return m, ref, e
}
func (s *Service) savePlan(ctx context.Context, p Principal, c commitRecord) error {
	b, e := json.Marshal(c.Plan)
	if e != nil {
		return e
	}
	_, e = s.db.Exec(ctx, "UPDATE studio.project_commits SET plan=$1,updated_at=now() WHERE organization_id=$2 AND project_id=$3 AND commit_id=$4 AND state='preparing'", b, p.OrganizationID, c.ProjectID, c.CommitID)
	return e
}
func (s *Service) terminal(ctx context.Context, p Principal, c commitRecord, state string, cause error) (Result, error) {
	tx, e := s.db.Begin(ctx)
	if e != nil {
		return Result{}, e
	}
	defer tx.Rollback(context.Background())
	_, e = tx.Exec(ctx, "UPDATE studio.project_commits SET state=$1,error_code=$2,updated_at=now() WHERE organization_id=$3 AND project_id=$4 AND commit_id=$5 AND state='preparing'", state, cause.Error(), p.OrganizationID, c.ProjectID, c.CommitID)
	if e != nil {
		return Result{}, e
	}
	if e = audit(ctx, tx, p, "project."+state, c.Revision); e != nil {
		return Result{}, e
	}
	if e = tx.Commit(ctx); e != nil {
		return Result{}, e
	}
	return Result{}, cause
}
func (s *Service) resume(ctx context.Context, p Principal, id, commit string) (Result, error) {
	select {
	case s.workers <- struct{}{}:
		defer func() { <-s.workers }()
	case <-ctx.Done():
		return Result{}, ctx.Err()
	}
	// Dedicated connection: an authority callback must not wait for a pool whose
	// connections are all held by commit workers. Closing also releases the lock.
	lock, e := pgx.ConnectConfig(ctx, s.db.Config().ConnConfig.Copy())
	if e != nil {
		return Result{}, e
	}
	defer lock.Close(context.Background())
	if _, e = lock.Exec(ctx, "SELECT pg_advisory_lock(hashtextextended($1,0))", "execute:"+p.OrganizationID+":"+id+":"+commit); e != nil {
		return Result{}, e
	}
	c, e := loadCommit(ctx, s.db, p.OrganizationID, id, commit)
	if e != nil {
		return Result{}, e
	}
	if c.Actor != p.SubjectID {
		return Result{}, ErrForbidden
	}
	if e = writable(ctx, s.db, p, id); e != nil {
		return Result{}, e
	}
	switch c.State {
	case "committed":
		return *c.Result, nil
	case "conflict":
		var current string
		if e = s.db.QueryRow(ctx, "SELECT head_revision_id::text FROM studio.projects WHERE organization_id=$1 AND project_id=$2", p.OrganizationID, id).Scan(&current); e != nil {
			return Result{}, e
		}
		return Result{}, &ConflictError{CurrentRevisionID: current}
	case "failed":
		return Result{}, errorCode(c.Error)
	}
	var head string
	if e = s.db.QueryRow(ctx, "SELECT COALESCE(head_revision_id::text,'') FROM studio.projects WHERE organization_id=$1 AND project_id=$2", p.OrganizationID, id).Scan(&head); e != nil {
		return Result{}, e
	}
	if head != c.Expected {
		return s.terminal(ctx, p, c, "conflict", &ConflictError{CurrentRevisionID: head})
	}
	for i, w := range c.Plan.Writes {
		if w.Content != nil {
			continue
		}
		v, err := s.content.Write(ctx, p, id, w)
		if err != nil {
			return Result{}, err
		}
		if !v.ContentRef.Valid() || v.OrganizationID != p.OrganizationID || v.Source.ProjectID != id {
			return Result{}, ErrDependency
		}
		c.Plan.Writes[i].Content = &v.ContentRef
		for j := range c.Plan.Manifest.Files {
			if c.Plan.Manifest.Files[j].ID == w.FileID {
				c.Plan.Manifest.Files[j].Content = v.ContentRef
			}
		}
		if e = s.savePlan(ctx, p, c); e != nil {
			return Result{}, e
		}
	}
	if c.Plan.Manifest.Validate() != nil {
		return s.terminal(ctx, p, c, "failed", ErrInvalid)
	}
	refs, e := s.checkContents(ctx, p, c.Plan.Manifest, c.Expected)
	if e != nil {
		if errors.Is(e, ErrInvalid) || errors.Is(e, ErrNotFound) {
			return s.terminal(ctx, p, c, "failed", e)
		}
		return Result{}, e
	}
	if c.Plan.ManifestRef == nil {
		b, err := json.Marshal(c.Plan.Manifest)
		if err != nil || len(b) > MaxManifestBytes {
			return s.terminal(ctx, p, c, "failed", ErrInvalid)
		}
		v, err := s.content.Write(ctx, p, id, TextWrite{WriteID: c.Plan.ManifestWriteID, Text: string(b), MIME: "application/json"})
		if err != nil {
			return Result{}, err
		}
		if !v.ContentRef.Valid() || v.OrganizationID != p.OrganizationID || v.Source.ProjectID != id {
			return Result{}, ErrDependency
		}
		c.Plan.ManifestRef = &v.ContentRef
	}
	refs = append(refs, *c.Plan.ManifestRef)
	c.Plan.Refs = uniqueRefs(refs)
	if e = s.savePlan(ctx, p, c); e != nil {
		return Result{}, e
	}
	if e = s.content.Retain(ctx, p, RetentionOwner{Kind: "project_revision", ID: c.Revision, CommitID: commit}, c.Plan.Refs); e != nil {
		return Result{}, e
	}
	return s.publish(ctx, p, c)
}
func errorCode(code string) error {
	for _, e := range []error{ErrInvalid, ErrNotFound, ErrForbidden, ErrConflict, ErrNotReady} {
		if code == e.Error() {
			return e
		}
	}
	return ErrDependency
}
func uniqueRefs(refs []ContentRef) []ContentRef {
	seen := map[ContentRef]bool{}
	out := []ContentRef{}
	for _, r := range refs {
		if !seen[r] {
			seen[r] = true
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.StoreID != b.StoreID {
			return a.StoreID < b.StoreID
		}
		if a.ArtifactID != b.ArtifactID {
			return a.ArtifactID < b.ArtifactID
		}
		return a.VersionID < b.VersionID
	})
	return out
}
func (s *Service) checkContents(ctx context.Context, p Principal, m Manifest, base string) ([]ContentRef, error) {
	refs := []ContentRef{}
	previous := map[ContentRef]bool{}
	if base != "" {
		old, _, e := revision(ctx, s.db, p.OrganizationID, m.ProjectID, base)
		if e != nil {
			return nil, e
		}
		for _, f := range old.Files {
			previous[f.Content] = true
		}
	}
	domains := map[string]bool{}
	for _, d := range m.DomainDocuments {
		domains[d.FileID] = true
	}
	for _, f := range m.Files {
		access := Access{}
		if previous[f.Content] {
			access = Access{ProjectID: m.ProjectID, RevisionID: base}
		}
		v, e := s.content.Metadata(ctx, p, f.Content, access)
		if e != nil {
			return nil, e
		}
		if v.ContentRef != f.Content || v.OrganizationID != p.OrganizationID {
			return nil, ErrDependency
		}
		mt, _, e := mime.ParseMediaType(v.MIME)
		if e != nil {
			return nil, ErrInvalid
		}
		if f.ID == m.EntryDocumentID || domains[f.ID] {
			if (f.ID == m.EntryDocumentID && mt != "text/markdown" && mt != "text/plain") || (domains[f.ID] && mt != "application/json") {
				return nil, ErrInvalid
			}
			data, e := s.content.Read(ctx, p, f.Content, access, MaxManifestBytes)
			if e != nil {
				return nil, e
			}
			if !utf8.Valid(data) || (domains[f.ID] && !json.Valid(data)) {
				return nil, ErrInvalid
			}
		}
		refs = append(refs, f.Content)
	}
	for _, a := range m.AssetRefs {
		if s.assets == nil {
			return nil, ErrDependency
		}
		more, e := s.assets.References(ctx, p, a)
		if e != nil {
			return nil, e
		}
		for _, r := range more {
			if !r.Valid() {
				return nil, ErrDependency
			}
		}
		refs = append(refs, more...)
	}
	if len(uniqueRefs(refs)) > MaxFiles {
		return nil, ErrInvalid
	}
	return refs, nil
}
func (s *Service) publish(ctx context.Context, p Principal, c commitRecord) (Result, error) {
	tx, e := s.db.Begin(ctx)
	if e != nil {
		return Result{}, e
	}
	defer tx.Rollback(context.Background())
	for _, a := range c.Plan.Manifest.AssetRefs {
		if _, e = assetReferences(ctx, tx, p, a, true); e != nil {
			return Result{}, e
		}
	}
	// Lock membership too: revocation cannot commit between this check and head publication.
	var member string
	e = tx.QueryRow(ctx, "SELECT role FROM studio.project_members WHERE organization_id=$1 AND project_id=$2 AND subject_id=$3 FOR SHARE", p.OrganizationID, c.ProjectID, p.SubjectID).Scan(&member)
	if errors.Is(e, pgx.ErrNoRows) || e == nil && (member != "owner" && member != "editor") {
		return Result{}, ErrForbidden
	}
	if e != nil {
		return Result{}, e
	}
	var head string
	e = tx.QueryRow(ctx, "SELECT COALESCE(head_revision_id::text,'') FROM studio.projects WHERE organization_id=$1 AND project_id=$2 FOR UPDATE", p.OrganizationID, c.ProjectID).Scan(&head)
	if e != nil {
		return Result{}, e
	}
	if head != c.Expected {
		_ = tx.Rollback(ctx)
		return s.terminal(ctx, p, c, "conflict", &ConflictError{CurrentRevisionID: head})
	}
	result := Result{ProjectID: c.ProjectID, RevisionID: c.Revision, ManifestRef: *c.Plan.ManifestRef, Manifest: c.Plan.Manifest, Invalidated: c.Plan.Invalidated}
	ref, _ := json.Marshal(result.ManifestRef)
	manifest, _ := json.Marshal(result.Manifest)
	data, _ := json.Marshal(result)
	_, e = tx.Exec(ctx, `INSERT INTO studio.project_revisions(revision_id,organization_id,project_id,parent_revision_id,manifest_ref,manifest_json,created_by,commit_id) VALUES($1,$2,$3,NULLIF($4,'')::uuid,$5,$6,$7,$8)`, c.Revision, p.OrganizationID, c.ProjectID, c.Expected, ref, manifest, p.SubjectID, c.CommitID)
	if e != nil {
		return Result{}, e
	}
	_, e = tx.Exec(ctx, "UPDATE studio.projects SET head_revision_id=$1,name=$2,category=$3,status=$4 WHERE organization_id=$5 AND project_id=$6", c.Revision, result.Manifest.Name, result.Manifest.Category, result.Manifest.Status, p.OrganizationID, c.ProjectID)
	if e != nil {
		return Result{}, e
	}
	_, e = tx.Exec(ctx, "UPDATE studio.project_commits SET state='committed',result=$1,updated_at=now() WHERE organization_id=$2 AND project_id=$3 AND commit_id=$4", data, p.OrganizationID, c.ProjectID, c.CommitID)
	if e != nil {
		return Result{}, e
	}
	if e = audit(ctx, tx, p, "project.commit", c.Revision); e != nil {
		return Result{}, e
	}
	if e = tx.Commit(ctx); e != nil {
		return Result{}, e
	}
	return result, nil
}
func (s *Service) Status(ctx context.Context, p Principal, id, commit string) (CommitStatus, error) {
	if !ValidID(commit) {
		return CommitStatus{}, ErrInvalid
	}
	member, e := role(ctx, s.db, p, id)
	if e != nil {
		return CommitStatus{}, e
	}
	c, e := loadCommit(ctx, s.db, p.OrganizationID, id, commit)
	if e != nil {
		return CommitStatus{}, e
	}
	if c.Actor != p.SubjectID && member != "owner" {
		return CommitStatus{}, ErrForbidden
	}
	status := CommitStatus{State: c.State, RequestHash: c.Hash, Result: c.Result, ErrorCode: c.Error}
	if c.State == "conflict" {
		if e = s.db.QueryRow(ctx, "SELECT head_revision_id::text FROM studio.projects WHERE organization_id=$1 AND project_id=$2", p.OrganizationID, id).Scan(&status.CurrentRevisionID); e != nil {
			return CommitStatus{}, e
		}
	}
	return status, nil
}
func (s *Service) Open(ctx context.Context, p Principal, id, selected string) (OpenResult, error) {
	if _, e := role(ctx, s.db, p, id); e != nil {
		return OpenResult{}, e
	}
	if selected != "" && !ValidID(selected) {
		return OpenResult{}, ErrInvalid
	}
	var head string
	e := s.db.QueryRow(ctx, "SELECT COALESCE(head_revision_id::text,'') FROM studio.projects WHERE organization_id=$1 AND project_id=$2", p.OrganizationID, id).Scan(&head)
	if e != nil {
		return OpenResult{}, e
	}
	if head == "" {
		return OpenResult{}, ErrNotReady
	}
	if selected == "" {
		selected = head
	}
	projection, ref, e := revision(ctx, s.db, p.OrganizationID, id, selected)
	if e != nil {
		return OpenResult{}, e
	}
	data, e := s.content.Read(ctx, p, ref, Access{ProjectID: id, RevisionID: selected}, MaxManifestBytes)
	if e != nil {
		return OpenResult{}, e
	}
	m, e := Decode(data)
	if e != nil || m.ProjectID != id {
		return OpenResult{}, ErrNotReady
	}
	a, _ := digest(m)
	b, _ := digest(projection)
	if a != b {
		return OpenResult{}, ErrNotReady
	}
	return OpenResult{Result: Result{ProjectID: id, RevisionID: selected, ManifestRef: ref, Manifest: m, Invalidated: []string{}}, HeadRevisionID: head}, nil
}
func (s *Service) List(ctx context.Context, p Principal, r ListRequest) (ListResult, error) {
	out := ListResult{Items: []ListItem{}}
	if !p.Valid() {
		return out, ErrForbidden
	}
	if r.Limit == 0 {
		r.Limit = 50
	}
	if r.Limit < 1 || r.Limit > 100 || (r.Cursor != "" && !ValidID(r.Cursor)) || (r.Category != "" && !tokenPattern.MatchString(r.Category)) {
		return out, ErrInvalid
	}
	rows, e := s.db.Query(ctx, `SELECT p.project_id::text,p.head_revision_id::text,p.name,p.category,p.status,p.created_at FROM studio.projects p JOIN studio.project_members m USING(organization_id,project_id) WHERE p.organization_id=$1 AND m.subject_id=$2 AND p.head_revision_id IS NOT NULL AND ($3='' OR p.category=$3) AND ($4='' OR p.project_id>NULLIF($4,'')::uuid) AND (NOT $6 OR m.role IN ('owner','editor')) ORDER BY p.project_id LIMIT $5`, p.OrganizationID, p.SubjectID, r.Category, r.Cursor, r.Limit+1, r.RequireWrite)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		var item ListItem
		if e = rows.Scan(&item.ProjectID, &item.HeadRevisionID, &item.Name, &item.Category, &item.Status, &item.CreatedAt); e != nil {
			return out, e
		}
		item.CreatedAt = item.CreatedAt.UTC()
		out.Items = append(out.Items, item)
	}
	if e = rows.Err(); e != nil {
		return out, e
	}
	if len(out.Items) > r.Limit {
		out.Items = out.Items[:r.Limit]
		out.NextCursor = out.Items[len(out.Items)-1].ProjectID
	}
	return out, nil
}
