package project

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/verdantflarehub/verdantflare-studio/internal/strictjson"
)

var (
	ErrApproval        = errors.New("GENERATION_APPROVAL_REQUIRED")
	ErrBudget          = errors.New("GENERATION_CALL_LIMIT")
	ErrGenerationState = errors.New("GENERATION_STATE_CONFLICT")
	sha256Pattern      = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// A plan is an immutable, bounded menu of exact attempts. It is not an approval.
type GenerationAttempt struct {
	OperationID    string          `json:"operation_id"`
	ObjectID       string          `json:"object_id"`
	StateID        string          `json:"state_id"`
	Purpose        string          `json:"purpose"`
	Tool           string          `json:"tool"`
	ServiceVersion string          `json:"service_version"`
	Arguments      json.RawMessage `json:"arguments"`
}
type GenerationPlan struct {
	ProjectID         string              `json:"project_id"`
	PlanID            string              `json:"plan_id"`
	RevisionID        string              `json:"revision_id"`
	PreparationSHA256 string              `json:"preparation_sha256"`
	Stage             string              `json:"stage"`
	CallLimit         int                 `json:"call_limit"`
	ExpiresAt         time.Time           `json:"expires_at"`
	Attempts          []GenerationAttempt `json:"attempts"`
}
type GenerationPlanKey struct {
	ProjectID string `json:"project_id"`
	PlanID    string `json:"plan_id"`
}
type GenerationDecision struct {
	GenerationPlanKey
	RequestSHA256 string `json:"request_sha256"`
}
type GenerationOperationKey struct {
	GenerationPlanKey
	OperationID string `json:"operation_id"`
}
type GenerationOperation struct {
	OperationID   string          `json:"operation_id"`
	State         string          `json:"state"`
	RequestSHA256 string          `json:"request_sha256"`
	Response      json.RawMessage `json:"response,omitempty"`
}
type GenerationPlanStatus struct {
	Plan           GenerationPlan        `json:"plan"`
	RequestSHA256  string                `json:"request_sha256"`
	State          string                `json:"state"`
	OccupiedCalls  int                   `json:"occupied_calls"`
	RemainingCalls int                   `json:"remaining_calls"`
	ApprovedBy     string                `json:"approved_by,omitempty"`
	Operations     []GenerationOperation `json:"operations"`
}
type GenerationReservation struct {
	Operation GenerationOperation `json:"operation"`
	Attempt   GenerationAttempt   `json:"attempt"`
	Acquired  bool                `json:"acquired"`
}

func (p GenerationPlan) Validate(now time.Time) error {
	if !ValidID(p.ProjectID) || !ValidID(p.PlanID) || !ValidID(p.RevisionID) || !sha256Pattern.MatchString(p.PreparationSHA256) || !tokenPattern.MatchString(p.Stage) || p.CallLimit < 1 || p.CallLimit > 100 || len(p.Attempts) < 1 || len(p.Attempts) > 100 || p.CallLimit > len(p.Attempts) || !p.ExpiresAt.After(now) || p.ExpiresAt.After(now.Add(24*time.Hour)) {
		return ErrInvalid
	}
	seen := map[string]bool{}
	for _, a := range p.Attempts {
		if !ValidID(a.OperationID) || seen[a.OperationID] || strings.TrimSpace(a.ObjectID) == "" || utf8.RuneCountInString(a.ObjectID) > 128 || strings.TrimSpace(a.StateID) == "" || utf8.RuneCountInString(a.StateID) > 128 || strings.TrimSpace(a.Purpose) == "" || utf8.RuneCountInString(a.Purpose) > 256 || utf8.RuneCountInString(a.ServiceVersion) == 0 || utf8.RuneCountInString(a.ServiceVersion) > 64 || (a.Tool != "image.create" && a.Tool != "image.edit") {
			return ErrInvalid
		}
		seen[a.OperationID] = true
		if _, err := strictjson.ReadValue(bytes.NewReader(a.Arguments), 64<<10); err != nil {
			return ErrInvalid
		}
		var args map[string]json.RawMessage
		if json.Unmarshal(a.Arguments, &args) != nil || args == nil {
			return ErrInvalid
		}
		// Scope and deduplication are exclusively assigned by the dispatcher.
		for _, key := range []string{"project_id", "idempotency_key", "approval", "approved", "authorization"} {
			if _, ok := args[key]; ok {
				return ErrInvalid
			}
		}
	}
	return nil
}

func generationHash(v any) (string, error) {
	// Preserve exact JSON numbers while normalizing object key order/whitespace.
	b, e := json.Marshal(v)
	if e != nil {
		return "", e
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var normalized any
	if e = dec.Decode(&normalized); e != nil {
		return "", e
	}
	return digest(normalized)
}

func generationKeyValid(k GenerationPlanKey) bool { return ValidID(k.ProjectID) && ValidID(k.PlanID) }

type GenerationPolicyRequest struct {
	ProjectID string `json:"project_id"`
}
type GenerationPolicy struct {
	Controlled bool `json:"controlled"`
}

func (s *Service) GenerationPolicy(ctx context.Context, p Principal, id string) (GenerationPolicy, error) {
	if e := writable(ctx, s.db, p, id); e != nil {
		return GenerationPolicy{}, e
	}
	var out GenerationPolicy
	e := s.db.QueryRow(ctx, `SELECT category='ai_short_drama' OR EXISTS(SELECT 1 FROM studio.generation_plans g WHERE g.organization_id=p.organization_id AND g.project_id=p.project_id) FROM studio.projects p WHERE organization_id=$1 AND project_id=$2`, p.OrganizationID, id).Scan(&out.Controlled)
	return out, e
}

func generationWriteLock(ctx context.Context, tx pgx.Tx, p Principal, id string, owner bool) error {
	if !p.Valid() || !ValidID(id) {
		return ErrForbidden
	}
	var value string
	e := tx.QueryRow(ctx, `SELECT role FROM studio.project_members WHERE organization_id=$1 AND project_id=$2 AND subject_id=$3 FOR SHARE`, p.OrganizationID, id, p.SubjectID).Scan(&value)
	if errors.Is(e, pgx.ErrNoRows) || e == nil && (value != "owner" && (owner || value != "editor")) {
		return ErrForbidden
	}
	return e
}

func (s *Service) GenerationPropose(ctx context.Context, p Principal, plan GenerationPlan) (GenerationPlanStatus, error) {
	if e := plan.Validate(time.Now()); e != nil {
		return GenerationPlanStatus{}, e
	}
	hash, e := generationHash(plan)
	if e != nil {
		return GenerationPlanStatus{}, ErrInvalid
	}
	tx, e := s.db.Begin(ctx)
	if e != nil {
		return GenerationPlanStatus{}, e
	}
	defer tx.Rollback(context.Background())
	if e = generationWriteLock(ctx, tx, p, plan.ProjectID, false); e != nil {
		return GenerationPlanStatus{}, e
	}
	if e = advisory(ctx, tx, "generation:"+p.OrganizationID+":"+plan.ProjectID+":"+plan.PlanID); e != nil {
		return GenerationPlanStatus{}, e
	}
	var old string
	e = tx.QueryRow(ctx, `SELECT request_sha256 FROM studio.generation_plans WHERE organization_id=$1 AND project_id=$2 AND plan_id=$3`, p.OrganizationID, plan.ProjectID, plan.PlanID).Scan(&old)
	if e == nil && old != hash {
		return GenerationPlanStatus{}, ErrIdempotency
	}
	if e != nil && !errors.Is(e, pgx.ErrNoRows) {
		return GenerationPlanStatus{}, e
	}
	if errors.Is(e, pgx.ErrNoRows) {
		// Pin an existing revision in this project; no mutation of the project head.
		var category string
		e = tx.QueryRow(ctx, `SELECT manifest_json->>'category' FROM studio.project_revisions WHERE organization_id=$1 AND project_id=$2 AND revision_id=$3`, p.OrganizationID, plan.ProjectID, plan.RevisionID).Scan(&category)
		if errors.Is(e, pgx.ErrNoRows) {
			return GenerationPlanStatus{}, ErrNotFound
		}
		if e != nil {
			return GenerationPlanStatus{}, e
		}
		if category != "ai_short_drama" {
			return GenerationPlanStatus{}, ErrInvalid
		}
		data, _ := json.Marshal(plan)
		_, e = tx.Exec(ctx, `INSERT INTO studio.generation_plans(organization_id,project_id,plan_id,revision_id,request_sha256,specification,call_limit,expires_at,created_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, p.OrganizationID, plan.ProjectID, plan.PlanID, plan.RevisionID, hash, data, plan.CallLimit, plan.ExpiresAt, p.SubjectID)
		if e != nil {
			return GenerationPlanStatus{}, e
		}
		if e = audit(ctx, tx, p, "generation.propose", plan.PlanID); e != nil {
			return GenerationPlanStatus{}, e
		}
	}
	if e = tx.Commit(ctx); e != nil {
		return GenerationPlanStatus{}, e
	}
	return s.GenerationStatus(ctx, p, GenerationPlanKey{plan.ProjectID, plan.PlanID})
}

func loadGenerationPlan(ctx context.Context, q queryer, p Principal, k GenerationPlanKey, lock bool) (GenerationPlanStatus, error) {
	var out GenerationPlanStatus
	var data []byte
	sql := `SELECT specification,request_sha256,state,occupied_calls,COALESCE(approved_by::text,'') FROM studio.generation_plans WHERE organization_id=$1 AND project_id=$2 AND plan_id=$3`
	if lock {
		sql += " FOR UPDATE"
	}
	e := q.QueryRow(ctx, sql, p.OrganizationID, k.ProjectID, k.PlanID).Scan(&data, &out.RequestSHA256, &out.State, &out.OccupiedCalls, &out.ApprovedBy)
	if errors.Is(e, pgx.ErrNoRows) {
		return out, ErrNotFound
	}
	if e != nil {
		return out, e
	}
	if json.Unmarshal(data, &out.Plan) != nil {
		return out, ErrNotReady
	}
	out.RemainingCalls = out.Plan.CallLimit - out.OccupiedCalls
	out.Operations = []GenerationOperation{}
	return out, nil
}

func (s *Service) GenerationStatus(ctx context.Context, p Principal, k GenerationPlanKey) (GenerationPlanStatus, error) {
	if !generationKeyValid(k) {
		return GenerationPlanStatus{}, ErrInvalid
	}
	if _, e := role(ctx, s.db, p, k.ProjectID); e != nil {
		return GenerationPlanStatus{}, e
	}
	tx, e := s.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if e != nil {
		return GenerationPlanStatus{}, e
	}
	defer tx.Rollback(context.Background())
	out, e := loadGenerationPlan(ctx, tx, p, k, false)
	if e != nil {
		return out, e
	}
	rows, e := tx.Query(ctx, `SELECT operation_id::text,state,request_sha256,response FROM studio.generation_operations WHERE organization_id=$1 AND project_id=$2 AND plan_id=$3 ORDER BY created_at,operation_id`, p.OrganizationID, k.ProjectID, k.PlanID)
	if e != nil {
		return out, e
	}
	for rows.Next() {
		var op GenerationOperation
		if e = rows.Scan(&op.OperationID, &op.State, &op.RequestSHA256, &op.Response); e != nil {
			rows.Close()
			return out, e
		}
		out.Operations = append(out.Operations, op)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, e
	}
	return out, tx.Commit(ctx)
}

// GenerationApprove is reachable ONLY through the dedicated step-up handler,
// never through the normal Project HTTP/MCP routing table.
func (s *Service) GenerationApprove(ctx context.Context, p Principal, k GenerationDecision) (GenerationPlanStatus, error) {
	if !generationKeyValid(k.GenerationPlanKey) || !sha256Pattern.MatchString(k.RequestSHA256) {
		return GenerationPlanStatus{}, ErrInvalid
	}
	tx, e := s.db.Begin(ctx)
	if e != nil {
		return GenerationPlanStatus{}, e
	}
	defer tx.Rollback(context.Background())
	if e = generationWriteLock(ctx, tx, p, k.ProjectID, true); e != nil {
		return GenerationPlanStatus{}, e
	}
	plan, e := loadGenerationPlan(ctx, tx, p, k.GenerationPlanKey, true)
	if e != nil {
		return GenerationPlanStatus{}, e
	}
	if plan.RequestSHA256 != k.RequestSHA256 {
		return GenerationPlanStatus{}, ErrIdempotency
	}
	if !plan.Plan.ExpiresAt.After(time.Now()) || plan.State == "revoked" {
		return GenerationPlanStatus{}, ErrApproval
	}
	if plan.State == "pending" {
		_, e = tx.Exec(ctx, `UPDATE studio.generation_plans SET state='approved',approved_by=$4,approved_at=now() WHERE organization_id=$1 AND project_id=$2 AND plan_id=$3`, p.OrganizationID, k.ProjectID, k.PlanID, p.SubjectID)
		if e != nil {
			return GenerationPlanStatus{}, e
		}
		if e = audit(ctx, tx, p, "generation.approve", k.PlanID); e != nil {
			return GenerationPlanStatus{}, e
		}
	}
	if e = tx.Commit(ctx); e != nil {
		return GenerationPlanStatus{}, e
	}
	return s.GenerationStatus(ctx, p, k.GenerationPlanKey)
}

func (s *Service) GenerationRevoke(ctx context.Context, p Principal, k GenerationPlanKey) (GenerationPlanStatus, error) {
	if !generationKeyValid(k) {
		return GenerationPlanStatus{}, ErrInvalid
	}
	tx, e := s.db.Begin(ctx)
	if e != nil {
		return GenerationPlanStatus{}, e
	}
	defer tx.Rollback(context.Background())
	if e = generationWriteLock(ctx, tx, p, k.ProjectID, false); e != nil {
		return GenerationPlanStatus{}, e
	}
	plan, e := loadGenerationPlan(ctx, tx, p, k, true)
	if e != nil {
		return GenerationPlanStatus{}, e
	}
	if plan.State != "revoked" {
		_, e = tx.Exec(ctx, `UPDATE studio.generation_plans SET state='revoked' WHERE organization_id=$1 AND project_id=$2 AND plan_id=$3`, p.OrganizationID, k.ProjectID, k.PlanID)
		if e != nil {
			return GenerationPlanStatus{}, e
		}
		if e = audit(ctx, tx, p, "generation.revoke", k.PlanID); e != nil {
			return GenerationPlanStatus{}, e
		}
	}
	if e = tx.Commit(ctx); e != nil {
		return GenerationPlanStatus{}, e
	}
	return s.GenerationStatus(ctx, p, k)
}

// Reserve serializes against approval/revocation and every other reservation.
// Replays never acquire a second dispatch right, including after a crash.
func (s *Service) GenerationReserve(ctx context.Context, p Principal, k GenerationOperationKey) (GenerationReservation, error) {
	var out GenerationReservation
	if !generationKeyValid(k.GenerationPlanKey) || !ValidID(k.OperationID) {
		return out, ErrInvalid
	}
	tx, e := s.db.Begin(ctx)
	if e != nil {
		return out, e
	}
	defer tx.Rollback(context.Background())
	if e = generationWriteLock(ctx, tx, p, k.ProjectID, false); e != nil {
		return out, e
	}
	plan, e := loadGenerationPlan(ctx, tx, p, k.GenerationPlanKey, true)
	if e != nil {
		return out, e
	}
	for _, a := range plan.Plan.Attempts {
		if a.OperationID == k.OperationID {
			out.Attempt = a
			break
		}
	}
	if out.Attempt.OperationID == "" {
		return out, ErrForbidden
	}
	hash, e := generationHash(out.Attempt)
	if e != nil {
		return out, ErrInvalid
	}
	var oldPlan string
	e = tx.QueryRow(ctx, `SELECT plan_id::text,operation_id::text,state,request_sha256,response FROM studio.generation_operations WHERE organization_id=$1 AND project_id=$2 AND operation_id=$3`, p.OrganizationID, k.ProjectID, k.OperationID).Scan(&oldPlan, &out.Operation.OperationID, &out.Operation.State, &out.Operation.RequestSHA256, &out.Operation.Response)
	if e == nil {
		if oldPlan != k.PlanID || out.Operation.RequestSHA256 != hash {
			return out, ErrIdempotency
		}
		return out, tx.Commit(ctx)
	}
	if !errors.Is(e, pgx.ErrNoRows) {
		return out, e
	}
	if plan.State != "approved" || !plan.Plan.ExpiresAt.After(time.Now()) {
		return out, ErrApproval
	}
	if plan.RemainingCalls < 1 {
		return out, ErrBudget
	}
	_, e = tx.Exec(ctx, `INSERT INTO studio.generation_operations(organization_id,project_id,plan_id,operation_id,request_sha256,state,created_by) VALUES($1,$2,$3,$4,$5,'reserved',$6) ON CONFLICT DO NOTHING`, p.OrganizationID, k.ProjectID, k.PlanID, k.OperationID, hash, p.SubjectID)
	if e != nil {
		return out, e
	}
	// A cross-plan race on a reused operation must not consume either plan twice.
	var insertedPlan string
	e = tx.QueryRow(ctx, `SELECT plan_id::text FROM studio.generation_operations WHERE organization_id=$1 AND project_id=$2 AND operation_id=$3`, p.OrganizationID, k.ProjectID, k.OperationID).Scan(&insertedPlan)
	if e != nil {
		return out, e
	}
	if insertedPlan != k.PlanID {
		return out, ErrIdempotency
	}
	_, e = tx.Exec(ctx, `UPDATE studio.generation_plans SET occupied_calls=occupied_calls+1 WHERE organization_id=$1 AND project_id=$2 AND plan_id=$3`, p.OrganizationID, k.ProjectID, k.PlanID)
	if e != nil {
		return out, e
	}
	if e = audit(ctx, tx, p, "generation.reserve", k.OperationID); e != nil {
		return out, e
	}
	out.Operation = GenerationOperation{OperationID: k.OperationID, State: "reserved", RequestSHA256: hash}
	out.Acquired = true
	return out, tx.Commit(ctx)
}

type GenerationTransition struct {
	GenerationOperationKey
	State    string          `json:"state"`
	Response json.RawMessage `json:"response,omitempty"`
}

// Only the internal dispatcher may transition an operation. Unknown is durable
// BEFORE a supplier call; an ambiguous request can never be released or retried.
func (s *Service) GenerationTransition(ctx context.Context, p Principal, k GenerationTransition) (GenerationOperation, error) {
	var out GenerationOperation
	if !generationKeyValid(k.GenerationPlanKey) || !ValidID(k.OperationID) || (k.State != "unknown" && k.State != "submitted" && k.State != "released") {
		return out, ErrInvalid
	}
	if (k.State == "submitted") != (len(k.Response) > 0) || len(k.Response) > 1<<20 {
		return out, ErrInvalid
	}
	if len(k.Response) > 0 {
		if _, e := strictjson.ReadValue(bytes.NewReader(k.Response), 1<<20); e != nil {
			return out, ErrInvalid
		}
	}
	tx, e := s.db.Begin(ctx)
	if e != nil {
		return out, e
	}
	defer tx.Rollback(context.Background())
	if e = generationWriteLock(ctx, tx, p, k.ProjectID, false); e != nil {
		return out, e
	}
	plan, e := loadGenerationPlan(ctx, tx, p, k.GenerationPlanKey, true)
	if e != nil {
		return out, e
	}
	e = tx.QueryRow(ctx, `SELECT operation_id::text,state,request_sha256,response FROM studio.generation_operations WHERE organization_id=$1 AND project_id=$2 AND plan_id=$3 AND operation_id=$4 FOR UPDATE`, p.OrganizationID, k.ProjectID, k.PlanID, k.OperationID).Scan(&out.OperationID, &out.State, &out.RequestSHA256, &out.Response)
	if errors.Is(e, pgx.ErrNoRows) {
		return out, ErrNotFound
	}
	if e != nil {
		return out, e
	}
	if out.State == k.State {
		before, _ := generationHash(out.Response)
		after, _ := generationHash(k.Response)
		if before != after {
			return out, ErrIdempotency
		}
		return out, tx.Commit(ctx)
	}
	if !((out.State == "reserved" && (k.State == "unknown" || k.State == "released")) || (out.State == "unknown" && k.State == "submitted")) {
		return out, ErrGenerationState
	}
	if k.State == "unknown" && (plan.State != "approved" || !plan.Plan.ExpiresAt.After(time.Now())) {
		return out, ErrApproval
	}
	_, e = tx.Exec(ctx, `UPDATE studio.generation_operations SET state=$5,response=$6,updated_at=now() WHERE organization_id=$1 AND project_id=$2 AND plan_id=$3 AND operation_id=$4`, p.OrganizationID, k.ProjectID, k.PlanID, k.OperationID, k.State, []byte(k.Response))
	if e != nil {
		return out, e
	}
	if k.State == "released" {
		_, e = tx.Exec(ctx, `UPDATE studio.generation_plans SET occupied_calls=occupied_calls-1 WHERE organization_id=$1 AND project_id=$2 AND plan_id=$3`, p.OrganizationID, k.ProjectID, k.PlanID)
		if e != nil {
			return out, e
		}
	}
	if e = audit(ctx, tx, p, "generation."+k.State, k.OperationID); e != nil {
		return out, e
	}
	out.State = k.State
	out.Response = k.Response
	return out, tx.Commit(ctx)
}
