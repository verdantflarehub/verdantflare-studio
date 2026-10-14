package application

import (
	"context"
	"encoding/json"
	"time"
)

type dashboardResourceBinding struct {
	PodUID    string `json:"pod_uid"`
	Namespace string `json:"namespace"`
	PodName   string `json:"pod_name"`
	Container string `json:"container"`
	GPUUUID   string `json:"gpu_uuid"`
}

type dashboardResourceMetric struct {
	Value     *float64 `json:"value"`
	Request   *float64 `json:"request"`
	Limit     *float64 `json:"limit"`
	SampledAt *string  `json:"sampled_at"`
	Quality   string   `json:"quality"`
	Reason    string   `json:"reason,omitempty"`
	Scope     string   `json:"scope"`
	Unit      string   `json:"unit"`
}

type dashboardWorkloads struct {
	SchemaVersion int    `json:"schema_version"`
	ObservedAt    string `json:"updated_at"`
	GPUSamples    []struct {
		UUID   string                  `json:"uuid"`
		Memory dashboardResourceMetric `json:"memory"`
	} `json:"gpu_samples"`
	Workloads []struct {
		PodUID        string `json:"pod_uid"`
		Namespace     string `json:"namespace"`
		PodName       string `json:"pod_name"`
		InstanceAlias string `json:"instance_alias"`
		CreatedAt     string `json:"created_at"`
		Containers    []struct {
			Name       string                  `json:"name"`
			CPU        dashboardResourceMetric `json:"cpu"`
			Memory     dashboardResourceMetric `json:"memory"`
			GPURequest *float64                `json:"gpu_request"`
			GPULimit   *float64                `json:"gpu_limit"`
		} `json:"containers"`
	} `json:"workloads"`
}

func dashboardTelemetryFresh(stamp string, now time.Time) bool {
	at, err := time.Parse(time.RFC3339Nano, stamp)
	return err == nil && !at.After(now.Add(5*time.Second)) && now.Sub(at) <= 30*time.Second
}

func safeDashboardMetric(m dashboardResourceMetric, unit string, now time.Time) dashboardResourceMetric {
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
	if m.Quality == "stale" || (m.Quality == "fresh" && validTime && !dashboardTelemetryFresh(*m.SampledAt, now)) {
		m.Value, m.Quality, m.Reason = nil, "stale", "SAMPLE_STALE"
	} else if m.Quality != "fresh" || !validTime || m.Value == nil || *m.Value < 0 {
		m.Value, m.Quality, m.Reason = nil, "unavailable", "METRICS_UNAVAILABLE"
	} else {
		m.Reason = ""
	}
	return m
}

func unavailableDashboardResources(item map[string]json.RawMessage, reason string) {
	var resources map[string]json.RawMessage
	if json.Unmarshal(item["resources"], &resources) != nil || resources == nil {
		resources = map[string]json.RawMessage{}
	}
	for key, unit := range map[string]string{"cpu": "cores", "memory": "bytes"} {
		resources[key], _ = json.Marshal(dashboardResourceMetric{Quality: "unavailable", Reason: reason, Unit: unit, Scope: "instance_container"})
	}
	if _, ok := resources["gpu"]; !ok {
		resources["gpu"], _ = json.Marshal(dashboardResourceMetric{Quality: "unavailable", Reason: reason, Unit: "bytes", Scope: "exclusive_device"})
	}
	item["resources"], _ = json.Marshal(resources)
	delete(item, "resources_observed_at")
}

func (d *Dashboards) instanceResourceView(ctx context.Context, token string, data []byte) ([]byte, bool) {
	var payload map[string]json.RawMessage
	if json.Unmarshal(data, &payload) != nil || payload == nil {
		return nil, false
	}
	var schemaVersion int
	var instances []map[string]json.RawMessage
	if json.Unmarshal(payload["schema_version"], &schemaVersion) != nil || schemaVersion != 1 || json.Unmarshal(payload["instances"], &instances) != nil || instances == nil {
		return nil, false
	}
	bindings := make([]dashboardResourceBinding, len(instances))
	needMetrics := false
	changed := false
	for i, item := range instances {
		if _, has := item["_resource_binding"]; has {
			_ = json.Unmarshal(item["_resource_binding"], &bindings[i])
			delete(item, "_resource_binding")
			changed = true
		}
		b := bindings[i]
		if b.PodUID != "" && b.Namespace != "" && b.PodName != "" && b.Container != "" {
			unavailableDashboardResources(item, "TELEMETRY_UNAVAILABLE")
			needMetrics = true
			changed = true
		}
	}
	if !changed {
		return data, true
	}
	var workloads dashboardWorkloads
	if needMetrics && d.station != nil {
		wctx, cancel := context.WithTimeout(ctx, 4*time.Second)
		defer cancel()
		result := d.station.Call(wctx, token, Request{Method: "GET", Path: "resources/workloads"})
		if result.Status == 200 {
			if json.Unmarshal(result.Data, &workloads) != nil {
				workloads = dashboardWorkloads{}
			}
		}
	}
	now := time.Now()
	if workloads.SchemaVersion == 2 && dashboardTelemetryFresh(workloads.ObservedAt, now) {
		for i, item := range instances {
			var alias, status, observed string
			_ = json.Unmarshal(item["alias"], &alias)
			_ = json.Unmarshal(item["status"], &status)
			_ = json.Unmarshal(item["observed_at"], &observed)
			b := bindings[i]
			if b.PodUID == "" || b.Namespace == "" || b.PodName == "" || b.Container == "" || alias == "" || status != "running" || !dashboardTelemetryFresh(observed, now) {
				continue
			}
			count := 0
			var cpu, memory dashboardResourceMetric
			wholeCard := false
			noGPURequested := false
			createdAt := ""
			for _, w := range workloads.Workloads {
				if w.PodUID != b.PodUID || w.Namespace != b.Namespace || w.PodName != b.PodName || (w.InstanceAlias != "" && w.InstanceAlias != alias) {
					continue
				}
				for _, container := range w.Containers {
					if container.Name == b.Container {
						count++
						cpu, memory = container.CPU, container.Memory
						wholeCard = container.GPURequest != nil && *container.GPURequest == 1 && container.GPULimit != nil && *container.GPULimit == 1
						noGPURequested = (container.GPURequest == nil || *container.GPURequest == 0) && (container.GPULimit == nil || *container.GPULimit == 0)
						createdAt = w.CreatedAt
					}
				}
			}
			if count != 1 {
				unavailableDashboardResources(item, "RESOURCE_BINDING_MISMATCH")
				continue
			}
			var resources map[string]json.RawMessage
			if json.Unmarshal(item["resources"], &resources) != nil || resources == nil {
				resources = map[string]json.RawMessage{}
			}
			safeCPU := safeDashboardMetric(cpu, "cores", now)
			safeMem := safeDashboardMetric(memory, "bytes", now)
			resources["cpu"], _ = json.Marshal(safeCPU)
			resources["memory"], _ = json.Marshal(safeMem)
			if wholeCard && b.GPUUUID != "" {
				matches := 0
				var gpu dashboardResourceMetric
				for _, sample := range workloads.GPUSamples {
					if sample.UUID == b.GPUUUID {
						matches++
						gpu = sample.Memory
					}
				}
				if matches == 1 {
					gpu = safeDashboardMetric(gpu, "bytes", now)
					created, createErr := time.Parse(time.RFC3339Nano, createdAt)
					if gpu.SampledAt != nil {
						sampled, sampleErr := time.Parse(time.RFC3339Nano, *gpu.SampledAt)
						if createErr != nil || sampleErr != nil || sampled.Before(created) {
							gpu.Value = nil
							gpu.Quality = "unavailable"
							gpu.Reason = "SAMPLE_IDENTITY_UNVERIFIED"
						}
					}
					gpu.Scope = "exclusive_gpu"
					resources["gpu"], _ = json.Marshal(gpu)
				}
				item["allocated_gpu_count"] = json.RawMessage("1")
			} else if noGPURequested && (safeMem.Quality == "fresh" || safeCPU.Quality == "fresh") {
				zero := 0.0
				sampledAt := safeMem.SampledAt
				if sampledAt == nil {
					sampledAt = safeCPU.SampledAt
				}
				resources["gpu"], _ = json.Marshal(dashboardResourceMetric{
					Value:     &zero,
					Request:   &zero,
					Limit:     &zero,
					SampledAt: sampledAt,
					Quality:   "fresh",
					Scope:     "exclusive_device",
					Unit:      "bytes",
				})
			}
			item["resources"], _ = json.Marshal(resources)
			item["resources_observed_at"], _ = json.Marshal(workloads.ObservedAt)
		}
	}
	payload["instances"], _ = json.Marshal(instances)
	out, err := json.Marshal(payload)
	if err != nil {
		return nil, false
	}
	return out, true
}
