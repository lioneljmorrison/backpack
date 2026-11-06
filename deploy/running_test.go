package deploy

import (
	"strings"
	"testing"
)

// running runs running.sh with Docker context ctx ("" leaves it unset), and a
// docker whose backpack service is up or not.
func running(t *testing.T, ctx string, up bool) (string, int) {
	t.Helper()
	bin := t.TempDir()
	ps := ""
	if up {
		ps = "echo 3f2a9c1d"
	}
	fake(t, bin, "docker", `case "$*" in *" ps "*backpack*) `+ps+`;; esac; exit 0`)
	var env []string
	if ctx != "" {
		env = append(env, "DOCKER_CONTEXT="+ctx)
	}
	return runScript(t, "running.sh", t.TempDir(), bin, env)
}

func TestRunningIsSilentWhenBackpackIsUp(t *testing.T) {
	if out, code := running(t, "remote", true); code != 0 || out != "" {
		t.Fatalf("exit %d, output %q", code, out)
	}
}

func TestRunningSaysWhichDockerItCheckedAndHowToFixIt(t *testing.T) {
	out, code := running(t, "remote", false)
	if code != 1 {
		t.Fatalf("exit %d, output %q", code, out)
	}
	for _, want := range []string{"remote", "make up", "BACKPACK_DOCKER_CONTEXT ?=", "local.mk", "docker context ls"} {
		if !strings.Contains(out, want) {
			t.Errorf("output %q lacks %q", out, want)
		}
	}
}

func TestRunningNamesTheDefaultDockerWhenNoContextIsSet(t *testing.T) {
	out, code := running(t, "", false)
	if code != 1 || !strings.Contains(out, "default") {
		t.Fatalf("exit %d, output %q", code, out)
	}
}
