package deploy

import (
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// freePorts returns n ports nothing is listening on.
func freePorts(t *testing.T, n int) []string {
	t.Helper()
	var ports []string
	for range n {
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		ports = append(ports, strconv.Itoa(l.Addr().(*net.TCPAddr).Port))
		l.Close()
	}
	return ports
}

// listen holds port open until the test ends.
func listen(t *testing.T, port string) {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:"+port)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
}

// forward runs forward.sh for ports, with Docker context ctx ("" leaves it
// unset) whose endpoint is host, and an ssh that records its arguments.
func forward(t *testing.T, ctx, host string, ports []string) (out string, code int, ssh string) {
	t.Helper()
	bin := t.TempDir()
	log := filepath.Join(bin, "ssh.log")
	fake(t, bin, "docker", `[ "$1 $2" = "context inspect" ] && echo "`+host+`"; exit 0`)
	fake(t, bin, "ssh", `echo "$*" > "`+log+`"`)
	var env []string
	if ctx != "" {
		env = append(env, "DOCKER_CONTEXT="+ctx)
	}
	out, code = runScript(t, "forward.sh", t.TempDir(), bin, env, ports...)
	b, _ := os.ReadFile(log)
	return out, code, strings.TrimSpace(string(b))
}

func TestForwardOpensATunnelForEveryPort(t *testing.T) {
	ports := freePorts(t, 3)
	out, code, ssh := forward(t, "remote", "ssh://lab-box", ports)
	if code != 0 {
		t.Fatalf("exit %d, output %q", code, out)
	}
	want := "-f -N -o ExitOnForwardFailure=yes"
	for _, p := range ports {
		want += " -L 127.0.0.1:" + p + ":127.0.0.1:" + p
	}
	want += " lab-box"
	if ssh != want {
		t.Fatalf("ssh %q, want %q", ssh, want)
	}
}

func TestForwardDoesNothingForALocalStack(t *testing.T) {
	out, code, ssh := forward(t, "", "", freePorts(t, 3))
	if code != 0 || ssh != "" || !strings.Contains(out, "nothing to forward") {
		t.Fatalf("exit %d, output %q, ssh %q", code, out, ssh)
	}
}

func TestForwardRefusesAContextThatIsNotReachedOverSSH(t *testing.T) {
	out, code, ssh := forward(t, "remote", "tcp://10.0.0.5:2376", freePorts(t, 3))
	if code != 1 || ssh != "" || !strings.Contains(out, "SSH") {
		t.Fatalf("exit %d, output %q, ssh %q", code, out, ssh)
	}
}

func TestForwardLeavesAnOpenTunnelAlone(t *testing.T) {
	ports := freePorts(t, 3)
	for _, p := range ports {
		listen(t, p)
	}
	out, code, ssh := forward(t, "remote", "ssh://lab-box", ports)
	if code != 0 || ssh != "" || !strings.Contains(out, "already open") {
		t.Fatalf("exit %d, output %q, ssh %q", code, out, ssh)
	}
}

func TestForwardRefusesWhenSomePortsAreTaken(t *testing.T) {
	ports := freePorts(t, 3)
	listen(t, ports[1])
	out, code, ssh := forward(t, "remote", "ssh://lab-box", ports)
	if code != 1 || ssh != "" || !strings.Contains(out, ports[1]) {
		t.Fatalf("exit %d, output %q, ssh %q", code, out, ssh)
	}
}
