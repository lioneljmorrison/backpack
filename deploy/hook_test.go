// Package deploy holds no Go code; its tests drive hook.sh, the session-start
// hook other projects copy in.
package deploy

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

const unreachable = "Backpack unreachable — orientation skipped\n"

// project makes a project directory whose .backpack.json points at url.
func project(t *testing.T, url string) string {
	t.Helper()
	dir := t.TempDir()
	config := `{ "workspace": "demo", "url": "` + url + `" }` + "\n"
	if err := os.WriteFile(filepath.Join(dir, ".backpack.json"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// runHook runs hook.sh in dir with token as BACKPACK_TOKEN ("" leaves it unset).
func runHook(t *testing.T, dir, token string) (string, int) {
	t.Helper()
	hook, err := filepath.Abs("hook.sh")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", hook)
	cmd.Dir = dir
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "CLAUDE_PROJECT_DIR=" + dir}
	if token != "" {
		cmd.Env = append(cmd.Env, "BACKPACK_TOKEN="+token)
	}
	out, err := cmd.Output()
	if exit, ok := err.(*exec.ExitError); ok {
		return string(out), exit.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	return string(out), 0
}

func TestHookPrintsWhereAreWeAsText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/workspaces/demo/status" || r.Header.Get("Authorization") != "Bearer bp_test" ||
			r.Header.Get("Accept") != "text/plain" {
			http.Error(w, "unexpected request", http.StatusBadRequest)
			return
		}
		w.Write([]byte("In progress:\n  S-1 Hook — building\nMerged, waiting for a release: 1 (latest: S-2 Done)\n"))
	}))
	defer srv.Close()

	out, code := runHook(t, project(t, srv.URL), "bp_test")
	if code != 0 || out != "In progress:\n  S-1 Hook — building\nMerged, waiting for a release: 1 (latest: S-2 Done)\n" {
		t.Fatalf("exit %d, output %q", code, out)
	}
}

func TestHookIsSilentInAProjectWithoutBackpack(t *testing.T) {
	out, code := runHook(t, t.TempDir(), "bp_test")
	if code != 0 || out != "" {
		t.Fatalf("exit %d, output %q", code, out)
	}
}

func TestHookWithoutATokenSaysUnreachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("should not be asked\n"))
	}))
	defer srv.Close()

	out, code := runHook(t, project(t, srv.URL), "")
	if code != 0 || out != unreachable {
		t.Fatalf("exit %d, output %q", code, out)
	}
}

func TestHookSaysUnreachableWhenTheServiceRefuses(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusNotFound, http.StatusInternalServerError} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "no", status)
		}))
		out, code := runHook(t, project(t, srv.URL), "bp_test")
		srv.Close()
		if code != 0 || out != unreachable {
			t.Errorf("status %d: exit %d, output %q", status, code, out)
		}
	}
}

func TestHookGivesUpOnASlowService(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
	}))
	defer srv.Close()
	defer close(release)

	start := time.Now()
	out, code := runHook(t, project(t, srv.URL), "bp_test")
	if code != 0 || out != unreachable {
		t.Fatalf("exit %d, output %q", code, out)
	}
	if took := time.Since(start); took > 5*time.Second {
		t.Fatalf("took %v; the hook must give up after 3 seconds", took)
	}
}

func TestHookSaysUnreachableWhenNothingIsListening(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()

	out, code := runHook(t, project(t, url), "bp_test")
	if code != 0 || out != unreachable {
		t.Fatalf("exit %d, output %q", code, out)
	}
}
