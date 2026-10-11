package studioremote

import "testing"

func TestAppCenterFixedRemotePath(t *testing.T) {
	if !Allowed("GET", "/studio/appcenter/v1/view") {
		t.Fatal("view unavailable to desktop")
	}
	for _, path := range []string{"/studio/appcenter/v1/view?stationId=other", "/appcenter/v1/facts", "/studio/appcenter/v1/../view", "/studio/appcenter/v1/view/"} {
		if Allowed("GET", path) {
			t.Fatal("unapproved path accepted")
		}
	}
	if Allowed("POST", "/studio/appcenter/v1/view") {
		t.Fatal("read endpoint accepts writes")
	}
}
