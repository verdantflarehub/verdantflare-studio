package workspacehttp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/verdantflarehub/verdantflare-studio/internal/project"
)

type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

// Tools uses the same user Bearer as calls, without another login or a write.
func (r *Remote) Tools(ctx context.Context) ([]Tool, error) {
	id := uuid.Must(uuid.NewV7()).String()
	body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "method": "tools/list"})
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	resp, err := r.request(ctx, "POST", "/mcp", "application/json", bytes.NewReader(body), int64(len(body)))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, statusError(resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, responseLimit+1))
	var envelope struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      string          `json:"id"`
		Error   json.RawMessage `json:"error"`
		Result  struct {
			Tools []Tool `json:"tools"`
		} `json:"result"`
	}
	if err != nil || len(data) > responseLimit || json.Unmarshal(data, &envelope) != nil || envelope.JSONRPC != "2.0" || envelope.ID != id || len(envelope.Error) > 0 || envelope.Result.Tools == nil {
		return nil, project.ErrDependency
	}
	return envelope.Result.Tools, nil
}
