package studioremote

import "testing"

func TestAppCenterCommandRoutes(t *testing.T) {
	for _, path := range []string{"/studio/appcenter/v1/commands", "/studio/appcenter/v1/commands/prepare"} {
		if !Allowed("POST", path) {
			t.Fatal("fixed route unavailable")
		}
		for _, method := range []string{"GET", "DELETE", "PUT", "PATCH"} {
			if Allowed(method, path) {
				t.Fatal("unexpected method")
			}
		}
		for _, suffix := range []string{"?scope=other", "/extra", "/../commands", "%2f"} {
			if Allowed("POST", path+suffix) {
				t.Fatal("unexpected path")
			}
		}
	}
}
