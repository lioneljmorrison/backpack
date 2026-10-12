package deploy

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// fake puts an executable script called name in bin, ahead of the real one.
func fake(t *testing.T, bin, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/usr/bin/env bash\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}

// runScript runs a deploy script in dir with bin first on the PATH and the
// extra environment given, and returns its combined output and exit code.
func runScript(t *testing.T, script, dir, bin string, env []string, args ...string) (string, int) {
	t.Helper()
	path, err := filepath.Abs(script)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", append([]string{path}, args...)...)
	cmd.Dir = dir
	cmd.Env = append([]string{"PATH=" + bin + ":" + os.Getenv("PATH")}, env...)
	out, err := cmd.CombinedOutput()
	if exit, ok := err.(*exec.ExitError); ok {
		return string(out), exit.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	return string(out), 0
}

// guard runs tunnel-guard.sh in dir, with a docker that reports the tunnel
// running on this host or not.
func guard(t *testing.T, dir string, tunnelHere bool) (string, int) {
	t.Helper()
	bin := t.TempDir()
	ps := ""
	if tunnelHere {
		ps = "echo 3f2a9c1d"
	}
	fake(t, bin, "docker", `case "$*" in *" ps "*cloudflared*) `+ps+`;; esac; exit 0`)
	return runScript(t, "tunnel-guard.sh", dir, bin, nil)
}

func healthy() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte("ok\n"))
	}))
}

func TestGuardRefusesWhenAnotherHostServesTheAddress(t *testing.T) {
	srv := healthy()
	defer srv.Close()

	out, code := guard(t, project(t, srv.URL), false)
	if code != 1 || !strings.Contains(out, "already served") || !strings.Contains(out, srv.URL) {
		t.Fatalf("exit %d, output %q", code, out)
	}
}

func TestGuardAllowsRestartingTheTunnelOnTheHostThatServesIt(t *testing.T) {
	srv := healthy()
	defer srv.Close()

	if out, code := guard(t, project(t, srv.URL), true); code != 0 {
		t.Fatalf("exit %d, output %q", code, out)
	}
}

func TestGuardAllowsStartingWhenTheAddressDoesNotAnswer(t *testing.T) {
	for _, status := range []int{http.StatusBadGateway, http.StatusNotFound} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "no", status)
		}))
		out, code := guard(t, project(t, srv.URL), false)
		srv.Close()
		if code != 0 {
			t.Errorf("status %d: exit %d, output %q", status, code, out)
		}
	}

	srv := healthy()
	url := srv.URL
	srv.Close()
	if out, code := guard(t, project(t, url), false); code != 0 {
		t.Fatalf("nothing listening: exit %d, output %q", code, out)
	}
}

func TestGuardAllowsAProjectWithoutAnAddress(t *testing.T) {
	if out, code := guard(t, t.TempDir(), false); code != 0 {
		t.Fatalf("exit %d, output %q", code, out)
	}
}
