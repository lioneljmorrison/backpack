package deploy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// resume runs tunnel-resume.sh in dir, with a docker that reports a stopped
// tunnel container or not (and never a running one, as the guard asks), and
// returns its output, exit code and every docker call it made.
func resume(t *testing.T, dir string, stopped bool) (string, int, string) {
	t.Helper()
	bin := t.TempDir()
	calls := filepath.Join(bin, "calls.log")
	exited := ""
	if stopped {
		exited = "echo 3f2a9c1d"
	}
	fake(t, bin, "docker", `echo "$*" >> `+calls+`
case "$*" in
*exited*cloudflared*) `+exited+`;;
*running*cloudflared*) ;;
esac
exit 0`)
	out, code := runScript(t, "tunnel-resume.sh", dir, bin, nil)
	log, _ := os.ReadFile(calls)
	return out, code, string(log)
}

func TestResumeDoesNothingWithoutAStoppedTunnel(t *testing.T) {
	out, code, log := resume(t, t.TempDir(), false)
	if code != 0 || out != "" || strings.Contains(log, " start ") {
		t.Fatalf("exit %d, output %q, docker calls %q", code, out, log)
	}
}

func TestResumeStartsAStoppedTunnel(t *testing.T) {
	srv := healthy()
	url := srv.URL
	srv.Close()

	out, code, log := resume(t, project(t, url), true)
	if code != 0 || !strings.Contains(log, "start cloudflared") {
		t.Fatalf("exit %d, output %q, docker calls %q", code, out, log)
	}
}

func TestResumeRefusesWhenAnotherHostServesTheAddress(t *testing.T) {
	srv := healthy()
	defer srv.Close()

	out, code, log := resume(t, project(t, srv.URL), true)
	if code != 1 || !strings.Contains(out, "already served") || !strings.Contains(out, "not started") ||
		strings.Contains(log, " start ") {
		t.Fatalf("exit %d, output %q, docker calls %q", code, out, log)
	}
}
