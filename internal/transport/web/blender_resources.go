package web

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/verdantflarehub/verdantflare-studio/internal/application"
)

type blenderResourceBinding struct {
	PodUID    string `json:"pod_uid"`
	Namespace string `json:"namespace"`
	PodName   string `json:"pod_name"`
	Container string `json:"container"`
}
type blenderResourceMetric struct {
	Value     *float64 `json:"value"`
	Request   *float64 `json:"request"`
	Limit     *float64 `json:"limit"`
	SampledAt *string  `json:"sampled_at"`
	Quality   string   `json:"quality"`
	Reason    string   `json:"reason,omitempty"`
	Scope     string   `json:"scope"`
	Unit      string   `json:"unit"`
}
type blenderWorkloads struct {
	SchemaVersion int    `json:"schema_version"`
	ObservedAt    string `json:"updated_at"`
	Workloads     []struct {
		PodUID        string `json:"pod_uid"`
		Namespace     string `json:"namespace"`
		PodName       string `json:"pod_name"`
		InstanceAlias string `json:"instance_alias"`
		Containers    []struct {
			Name   string                `json:"name"`
			CPU    blenderResourceMetric `json:"cpu"`
			Memory blenderResourceMetric `json:"memory"`
		} `json:"containers"`
	} `json:"workloads"`
}

func telemetryFresh(stamp string, now time.Time) bool {
	at, err := time.Parse(time.RFC3339Nano, stamp)
	return err == nil && !at.After(now.Add(5*time.Second)) && now.Sub(at) <= 30*time.Second
}

func safeBlenderMetric(m blenderResourceMetric, unit string, now time.Time) blenderResourceMetric {
	// Copy only public fields. Do not forward arbitrary upstream error text or metadata.
	m.Unit, m.Scope = unit, "instance_container"
	if m.Request != nil && *m.Request < 0 {
		m.Request = nil
	}
	if m.Limit != nil && *m.Limit < 0 {
		m.Limit = nil
	}
	validTime := false
	if m.SampledAt != nil {
		at, err := time.Parse(time.RFC3339Nano, *m.SampledAt)
		validTime = err == nil && !at.After(now.Add(5*time.Second))
		if !validTime {
			m.SampledAt = nil
		}
	}
	if m.Quality == "stale" || (m.Quality == "fresh" && validTime && !telemetryFresh(*m.SampledAt, now)) {
		m.Value, m.Quality, m.Reason = nil, "stale", "SAMPLE_STALE"
	} else if m.Quality != "fresh" || !validTime || m.Value == nil || *m.Value < 0 {
		m.Value, m.Quality, m.Reason = nil, "unavailable", "METRICS_UNAVAILABLE"
	} else {
		m.Reason = ""
	}
	return m
}

func unavailableBlenderResources(item map[string]json.RawMessage, reason string) {
	var resources map[string]json.RawMessage
	if json.Unmarshal(item["resources"], &resources) != nil || resources == nil {
		return
	}
	for key, unit := range map[string]string{"cpu": "cores", "memory": "bytes"} {
		resources[key], _ = json.Marshal(blenderResourceMetric{Quality: "unavailable", Reason: reason, Unit: unit, Scope: "instance_container"})
	}
	item["resources"], _ = json.Marshal(resources)
	delete(item, "resources_observed_at")
}

// The app has already authorized each instance. Core data never reaches the
// browser wholesale; join using the worker's authenticated, private identity.
func (s *Server) blenderResourceView(c *gin.Context, data []byte) ([]byte, error) {
	var payload struct {
		SchemaVersion int                          `json:"schema_version"`
		Capabilities  json.RawMessage              `json:"capabilities"`
		Instances     []map[string]json.RawMessage `json:"instances"`
	}
	if json.Unmarshal(data, &payload) != nil || payload.SchemaVersion != 1 || payload.Instances == nil {
		return nil, fmt.Errorf("invalid instance directory")
	}
	bindings := make([]blenderResourceBinding, len(payload.Instances))
	needMetrics := false
	for i, item := range payload.Instances {
		_ = json.Unmarshal(item["_resource_binding"], &bindings[i])
		delete(item, "_resource_binding")
		b := bindings[i]
		if b.PodUID != "" && b.Namespace != "" && b.PodName != "" && b.Container != "" {
			unavailableBlenderResources(item, "TELEMETRY_UNAVAILABLE")
		}
		needMetrics = needMetrics || (b.PodUID != "" && b.Namespace != "" && b.PodName != "" && b.Container != "")
	}
	var workloads blenderWorkloads
	if needMetrics {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 4*time.Second)
		defer cancel()
		result := s.station.Call(ctx, c.GetString("verifiedStationToken"), application.Request{Method: "GET", Path: "resources/workloads"})
		if result.Status == 200 {
			if json.Unmarshal(result.Data, &workloads) != nil {
				workloads = blenderWorkloads{}
			}
		}
	}
	now := time.Now()
	if workloads.SchemaVersion == 2 && telemetryFresh(workloads.ObservedAt, now) {
		for i, item := range payload.Instances {
			var alias, status, observed string
			_ = json.Unmarshal(item["alias"], &alias)
			_ = json.Unmarshal(item["status"], &status)
			_ = json.Unmarshal(item["observed_at"], &observed)
			b := bindings[i]
			if b.PodUID == "" || b.Namespace == "" || b.PodName == "" || b.Container == "" || alias == "" || status != "running" || !telemetryFresh(observed, now) {
				continue
			}
			count := 0
			var cpu, memory blenderResourceMetric
			for _, w := range workloads.Workloads {
				if w.PodUID != b.PodUID || w.Namespace != b.Namespace || w.PodName != b.PodName || w.InstanceAlias != alias {
					continue
				}
				for _, container := range w.Containers {
					if container.Name == b.Container {
						count++
						cpu, memory = container.CPU, container.Memory
					}
				}
			}
			if count != 1 {
				unavailableBlenderResources(item, "RESOURCE_BINDING_MISMATCH")
				continue
			}
			var resources map[string]json.RawMessage
			if json.Unmarshal(item["resources"], &resources) != nil || resources == nil {
				continue
			}
			resources["cpu"], _ = json.Marshal(safeBlenderMetric(cpu, "cores", now))
			resources["memory"], _ = json.Marshal(safeBlenderMetric(memory, "bytes", now))
			item["resources"], _ = json.Marshal(resources)
			item["resources_observed_at"], _ = json.Marshal(workloads.ObservedAt)
		}
	}
	return json.Marshal(payload)
}
