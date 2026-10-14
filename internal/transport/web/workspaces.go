package web

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/verdantflarehub/verdantflare-studio/internal/application"
	"github.com/verdantflarehub/verdantflare-studio/internal/project"
)

func (s *Server) fetchBlenderInstanceNames(ctx context.Context, p project.Principal) (map[string]string, bool) {
	u, ok := s.blenderOrigin()
	token := os.Getenv("STUDIO_BLENDER_SERVICE_TOKEN")
	if !ok || token == "" {
		return nil, false
	}
	u.Path = "/internal/instances"
	reqCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, false
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-User-Id", p.SubjectID)
	req.Header.Set("X-Organization-Id", p.OrganizationID)
	req.Header.Set("X-Request-Id", p.RequestID)
	req.Header.Set("Accept", "application/json")
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 4 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return nil, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, false
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, (1<<20)+1))
	if err != nil || len(raw) > 1<<20 {
		return nil, false
	}
	var payload struct {
		Instances []struct {
			InstanceID  string `json:"instance_id"`
			Name        string `json:"name"`
			ProjectName string `json:"project_name"`
		} `json:"instances"`
	}
	if json.Unmarshal(raw, &payload) != nil || payload.Instances == nil {
		return nil, false
	}
	names := make(map[string]string, len(payload.Instances))
	for _, item := range payload.Instances {
		if item.InstanceID != "" {
			label := strings.TrimSpace(item.Name)
			if label == "" {
				label = strings.TrimSpace(item.ProjectName)
			}
			names[item.InstanceID] = label
		}
	}
	return names, true
}

func (s *Server) fetchComfyUIInstanceNames(ctx context.Context, p project.Principal) (map[string]string, bool) {
	u, ok := s.comfyuiOrigin()
	token := os.Getenv("STUDIO_COMFYUI_SERVICE_TOKEN")
	if !ok || len(token) < 32 || strings.ContainsAny(token, " \t\r\n") {
		return nil, false
	}
	u.Path = "/internal/instances"
	reqCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, false
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-User-Id", p.SubjectID)
	req.Header.Set("X-Organization-Id", p.OrganizationID)
	req.Header.Set("X-Request-Id", p.RequestID)
	req.Header.Set("Accept", "application/json")
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 4 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return nil, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, false
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, (1<<20)+1))
	if err != nil || len(raw) > 1<<20 {
		return nil, false
	}
	var payload struct {
		Instances []struct {
			InstanceID string `json:"instance_id"`
			Name       string `json:"name"`
		} `json:"instances"`
	}
	if json.Unmarshal(raw, &payload) != nil || payload.Instances == nil {
		return nil, false
	}
	names := make(map[string]string, len(payload.Instances))
	for _, item := range payload.Instances {
		if item.InstanceID != "" {
			names[item.InstanceID] = strings.TrimSpace(item.Name)
		}
	}
	return names, true
}

func (s *Server) enrichWorkspaces(ctx context.Context, token string, raw []byte) []byte {
	var payload struct {
		Workspaces []map[string]any `json:"workspaces"`
		RequestID  string           `json:"request_id"`
	}
	if json.Unmarshal(raw, &payload) != nil || payload.Workspaces == nil {
		return raw
	}
	p, status := s.station.Principal(ctx, token, uuid.Must(uuid.NewV7()).String())
	if status != http.StatusOK {
		return raw
	}
	var (
		wg           sync.WaitGroup
		blenderNames map[string]string
		comfyuiNames map[string]string
		comfyuiOK    bool
	)
	wg.Add(2)
	go func() {
		defer wg.Done()
		blenderNames, _ = s.fetchBlenderInstanceNames(ctx, p)
	}()
	go func() {
		defer wg.Done()
		comfyuiNames, comfyuiOK = s.fetchComfyUIInstanceNames(ctx, p)
	}()
	wg.Wait()

	for _, ws := range payload.Workspaces {
		ns, _ := ws["namespace"].(string)
		instID, _ := ws["instance_id"].(string)
		state, _ := ws["state"].(string)
		if instID == "" {
			continue
		}
		switch ns {
		case "verdantflare-blender":
			if name, exists := blenderNames[instID]; exists && name != "" {
				ws["instance_name"] = name
			} else if state == "retained" {
				if cur, _ := ws["instance_name"].(string); cur == "" {
					ws["instance_name"] = "Blender 历史实例（已销毁）"
				}
			}
		case "verdantflare-comfyui":
			if name, exists := comfyuiNames[instID]; exists {
				if name != "" {
					ws["instance_name"] = name
				}
			} else if comfyuiOK && state == "bound_stopped" {
				ws["state"] = "retained"
				ws["allowed_actions"] = []string{"release"}
				ws["instance_name"] = "ComfyUI 实例（已销毁）"
				if retAt, _ := ws["retained_at"].(string); retAt == "" {
					if boundAt, _ := ws["bound_at"].(string); boundAt != "" {
						ws["retained_at"] = boundAt
					}
				}
			}
		}
	}
	enriched, err := json.Marshal(payload)
	if err != nil {
		return raw
	}
	return enriched
}

func (s *Server) guardComfyUIWorkspaceRelease(ctx context.Context, token string, body []byte) *application.Result {
	var req struct {
		Namespace string `json:"namespace"`
		PVCName   string `json:"pvc_name"`
	}
	if json.Unmarshal(body, &req) != nil || req.Namespace != "verdantflare-comfyui" || req.PVCName == "" || token == "" {
		return nil
	}
	listRes := s.station.Call(ctx, token, application.Request{Path: "resources/workspaces", Method: "GET"})
	if listRes.Status != http.StatusOK {
		return nil
	}
	var payload struct {
		Workspaces []struct {
			Namespace  string `json:"namespace"`
			PVCName    string `json:"pvc_name"`
			InstanceID string `json:"instance_id"`
			State      string `json:"state"`
		} `json:"workspaces"`
	}
	if json.Unmarshal(listRes.Data, &payload) != nil {
		return nil
	}
	var boundInstanceID string
	for _, ws := range payload.Workspaces {
		if ws.Namespace == "verdantflare-comfyui" && ws.PVCName == req.PVCName {
			if ws.State == "in_use" {
				res := application.Error(http.StatusConflict, "WORKSPACE_IN_USE")
				return &res
			}
			boundInstanceID = ws.InstanceID
			break
		}
	}
	if boundInstanceID == "" {
		return nil
	}
	p, status := s.station.Principal(ctx, token, uuid.Must(uuid.NewV7()).String())
	if status != http.StatusOK {
		res := application.Error(status, "SERVICE_UNAVAILABLE")
		return &res
	}
	comfyuiNames, comfyuiOK := s.fetchComfyUIInstanceNames(ctx, p)
	if !comfyuiOK {
		res := application.Error(http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE")
		return &res
	}
	if _, stillActive := comfyuiNames[boundInstanceID]; stillActive {
		b, _ := json.Marshal(map[string]string{
			"code":       "INSTANCE_NOT_DESTROYED",
			"message":    "Instance has not been destroyed; destroy the instance in App Dashboard before releasing its workspace volume",
			"request_id": p.RequestID,
		})
		return &application.Result{Status: http.StatusConflict, Data: b, RequestID: p.RequestID}
	}
	return nil
}
