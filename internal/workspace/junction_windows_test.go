package workspace

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

func TestWindowsJunctionDoesNotEscape(t *testing.T) {
	f := fixture()
	w, dir := openFixture(t, f)
	outside := t.TempDir()
	link := filepath.Join(dir, "images")
	system, e := windows.GetSystemDirectory()
	if e != nil {
		t.Fatal(e)
	}
	cmd := exec.Command(filepath.Join(system, "cmd.exe"), "/c", "mklink", "/J", link, outside)
	if b, e := cmd.CombinedOutput(); e != nil {
		t.Fatalf("create test junction: %v %s", e, b)
	}
	t.Cleanup(func() {
		if e := os.Remove(link); e != nil {
			t.Error(e)
		}
	})
	if e := w.Fetch(context.Background(), f.head.Manifest.Files[1].ID, 8<<20); !errors.Is(e, ErrConflict) {
		t.Fatal("junction followed", e)
	}
	entries, e := os.ReadDir(outside)
	if e != nil || len(entries) != 0 {
		t.Fatal("wrote outside chosen root", e)
	}
	if other, e := Open(context.Background(), link, "local-test", f.head.ProjectID, f); !errors.Is(e, ErrConflict) {
		if other != nil {
			other.Close()
		}
		t.Fatal("junction accepted as selected root", e)
	}
}
