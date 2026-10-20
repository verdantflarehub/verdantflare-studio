package mcp

import (
	"net/http"
	"net/url"
	"os"
	"time"
)

func publishingControlEndpoint() (*url.URL, bool) {
	u, err := url.Parse(os.Getenv("STUDIO_APP_PUBLISH_ENDPOINT"))
	return u, err == nil && u.Scheme == "https" && u.Host == "hub.verdantflarehub.com" && u.Path == "/api/control/internal/studio-publish/mcp" && u.User == nil && u.RawQuery == "" && u.Fragment == ""
}

func publishingRelayEndpoint() (*url.URL, bool) {
	u, err := url.Parse(os.Getenv("STUDIO_APP_PUBLISH_RELAY_URL"))
	return u, err == nil && u.Scheme == "http" && u.Host != "" && u.Path == "/mcp" && u.User == nil && u.RawQuery == "" && u.Fragment == ""
}

func publishingConfigured() bool {
	_, controlOK := publishingControlEndpoint()
	_, relayOK := publishingRelayEndpoint()
	return controlOK && relayOK && len(os.Getenv("STUDIO_APP_PUBLISH_HMAC_KEY")) >= 32
}

func publishingReady() bool {
	if !publishingConfigured() {
		return false
	}
	u, _ := publishingRelayEndpoint()
	ready := *u
	ready.Path = "/readyz"
	client := &http.Client{Timeout: 750 * time.Millisecond, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Get(ready.String())
	if err != nil {
		return false
	}
	defer response.Body.Close()
	return response.StatusCode == http.StatusNoContent
}

func publicationTools() []ToolDefinition {
	stringField := map[string]any{"type": "string"}
	stringsField := map[string]any{"type": "array", "items": stringField}
	showcase := map[string]any{"type": "object", "properties": map[string]any{
		"screenshots": stringsField, "highlights": stringsField, "whatsNew": stringField,
		"permissions": stringsField, "websiteUrl": stringField, "docsUrl": stringField,
		"sourceUrl": stringField, "license": stringField, "languages": stringField, "platforms": stringField,
	}, "required": []string{"screenshots", "highlights"}}
	chart := map[string]any{"type": "object", "properties": map[string]any{
		"ociRef": stringField, "version": stringField, "manifestDigest": stringField, "packageDigest": stringField,
	}, "required": []string{"ociRef", "version", "manifestDigest", "packageDigest"}}
	version := map[string]any{"type": "object", "properties": map[string]any{
		"version": stringField, "chart": chart, "releaseNotes": stringField, "sourceRevision": stringField,
		"upstreamVersion": stringField, "publisher": stringField, "sourceUrl": stringField,
		"licenseId": stringField, "licenseUrl": stringField,
	}, "required": []string{"version", "chart", "sourceRevision", "licenseId", "licenseUrl"}}
	publication := map[string]any{"type": "object", "properties": map[string]any{
		"appId": stringField, "name": stringField, "groupId": stringField, "category": stringField,
		"summary": stringField, "description": stringField, "developer": stringField,
		"brand": stringField, "iconRef": stringField, "publicIconUrl": stringField,
		"sortOrder": map[string]any{"type": "integer"}, "showcase": showcase, "version": version,
		"organizationIds": stringsField, "expectedRevision": map[string]any{"type": "integer"},
	}, "required": []string{"appId", "name", "groupId", "category", "summary", "description", "developer", "showcase", "version", "organizationIds"}}
	return []ToolDefinition{
		{Name: "app_publish.list", Description: "列出 Control 中的应用资料及当前 Station 目录回读，用于管理员查看发布状态与详情。", InputSchema: map[string]any{"type": "object", "properties": map[string]any{}}},
		{Name: "app_publish.organizations", Description: "列出当前可选择的目标组织及其 Control ID；需要应用运营管理员权限。", InputSchema: map[string]any{"type": "object", "properties": map[string]any{}}},
		{Name: "app_publish.get", Description: "按应用 ID 读取 Hub/Control 的现有目录资料和修订；更新前先读取。", InputSchema: map[string]any{"type": "object", "properties": map[string]any{"app_id": stringField}, "required": []string{"app_id"}}},
		{Name: "app_publish.readiness", Description: "查询当前应用的上架必填字段缺口。", InputSchema: map[string]any{"type": "object", "properties": map[string]any{"app_id": stringField}, "required": []string{"app_id"}}},
		{Name: "app_publish.publish", Description: "使用已发行 OCI Chart 和双摘要，把完整应用资料、固定版本及组织权益一次发布到 Station 目录；不会安装应用。源码 Chart 上传暂不在第一版。", InputSchema: publication},
	}
}
