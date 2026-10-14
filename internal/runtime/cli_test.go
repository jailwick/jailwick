package runtime

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	goruntime "runtime"
	"slices"
	"strings"
	"testing"

	"github.com/jailwick/jailwick/internal/spec"
)

func TestComposeFile(t *testing.T) {
	d := newCLI("docker", "docker", "/state")
	for sandbox, want := range map[string]string{
		"jailwick-api":         "projects/api/compose.yml",
		"jailwick-api--k3x9q2": "projects/api/sessions/k3x9q2.yml",
	} {
		if got := d.composeFile(sandbox); got != filepath.Join("/state", want) {
			t.Errorf("composeFile(%q) = %q", sandbox, got)
		}
	}
}

func TestBuildArgs(t *testing.T) {
	got := strings.Join(buildArgs(BuildOptions{
		Context: "/ctx", Tag: "jailwick-sandbox:abc", Platform: "linux/amd64", Pull: true, NoCache: true,
		Args: map[string]string{"B": "2", "A": "1"}, Labels: map[string]string{"dev.jailwick.built-by": "jailwick"},
	}), " ")
	want := "build --progress=plain -t jailwick-sandbox:abc --pull --no-cache --platform linux/amd64 --build-arg A=1 --build-arg B=2 " +
		"--label dev.jailwick.built-by=jailwick /ctx"
	if got != want {
		t.Fatalf("build args:\n got %s\nwant %s", got, want)
	}
}

// One service started again is recreated alone; never with --no-recreate, which compose refuses
// alongside --force-recreate.
func TestUpArgs(t *testing.T) {
	cases := []struct {
		opts UpOptions
		want string
	}{
		{UpOptions{}, "--no-recreate"},
		{UpOptions{Recreate: true, RemoveOrphans: true}, "--remove-orphans"},
		{UpOptions{Only: []string{"egress-proxy"}}, "--force-recreate --no-deps egress-proxy"},
		{UpOptions{Recreate: true, Only: []string{"postgres"}}, "--force-recreate --no-deps postgres"},
	}
	for _, c := range cases {
		got := strings.Join(upArgs("/s/compose.yml", "jailwick-api", c.opts), " ")
		if want := "compose -f /s/compose.yml -p jailwick-api up -d --quiet-pull " + c.want; got != want {
			t.Errorf("%+v:\n got %s\nwant %s", c.opts, got, want)
		}
	}
}

// `info` is slow: Check's answer serves every later Info. A failed one is asked again.
func TestInfoIsAskedOnce(t *testing.T) {
	if goruntime.GOOS == "windows" {
		t.Skip("a shell script stands in for docker")
	}
	dir := t.TempDir()
	calls, down := filepath.Join(dir, "calls"), filepath.Join(dir, "down")
	bin := filepath.Join(dir, "docker")
	script := `#!/bin/sh
echo "$1" >> ` + calls + `
[ "$1" = info ] && [ -e ` + down + ` ] && { echo "Cannot connect" >&2; exit 1; }
[ "$1" = info ] && echo '{"ServerVersion":"28.1","OperatingSystem":"OrbStack","Architecture":"aarch64"}'
exit 0
`
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	count := func(cmd string) int {
		data, _ := os.ReadFile(calls)
		return strings.Count(string(data), cmd+"\n")
	}
	ctx := context.Background()

	os.WriteFile(down, nil, 0o644)
	d := newCLI("docker", bin, dir)
	if err := d.Check(ctx); err == nil || !strings.Contains(err.Error(), "not running") {
		t.Fatalf("a stopped runtime: %v", err)
	}
	os.Remove(down)
	if err := d.Check(ctx); err != nil {
		t.Fatal(err)
	}
	i := d.Info(ctx)
	d.Info(ctx)
	if i.Version != "28.1" || i.Arch != "arm64" || i.HostSuffix != ".orb.local" {
		t.Fatalf("info: %+v", i)
	}
	if count("info") != 2 || count("compose") != 2 {
		t.Fatalf("info asked %d times, compose %d", count("info"), count("compose"))
	}
}

// Every runtime command runs without the Docker CLI's "What's next" hints (shown after a session).
func TestCommandsWithoutHints(t *testing.T) {
	cmd := newCLI("docker", "docker", "/state").command(context.Background(), map[string]string{"A": "1"}, "exec", "x")
	if !slices.Contains(cmd.Env, "DOCKER_CLI_HINTS=false") || !slices.Contains(cmd.Env, "A=1") {
		t.Fatalf("env: %v", cmd.Env[len(cmd.Env)-2:])
	}
}

// The app's volumes are created before compose up when missing, and only then.
func TestUpCreatesAppVolumes(t *testing.T) {
	if goruntime.GOOS == "windows" {
		t.Skip("a shell script stands in for docker")
	}
	dir := t.TempDir()
	calls, exists := filepath.Join(dir, "calls"), filepath.Join(dir, "exists")
	bin := filepath.Join(dir, "docker")
	script := `#!/bin/sh
echo "$*" >> ` + calls + `
[ "$1 $2" = "volume inspect" ] && { [ -e ` + exists + ` ] || exit 1; }
[ "$1 $2" = "volume create" ] && touch ` + exists + `
exit 0
`
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	sb := &spec.Sandbox{Name: "jailwick-api", Project: "api", AppVolumes: []string{spec.LoginVolume}}
	d := newCLI("docker", bin, dir)
	for range 2 {
		if err := d.Up(context.Background(), sb, UpOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	data, _ := os.ReadFile(calls)
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	want := []string{
		"volume inspect " + spec.LoginVolume, "volume create " + spec.LoginVolume, "compose",
		"volume inspect " + spec.LoginVolume, "compose",
	}
	if len(lines) != len(want) {
		t.Fatalf("calls:\n%s", data)
	}
	for i, w := range want {
		if !strings.HasPrefix(lines[i], w) {
			t.Fatalf("call %d: %q, want %q\n%s", i, lines[i], w, data)
		}
	}
}

// A failed command says what the runtime said, even when nobody kept its output.
func TestStreamFailureSaysWhy(t *testing.T) {
	if goruntime.GOOS == "windows" {
		t.Skip("a shell script stands in for docker")
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "docker")
	script := "#!/bin/sh\necho 'Container jailwick-x-sandbox-1  Creating'\necho 'Error response from daemon: invalid mount config: bind source path does not exist: /Users/me/missing' >&2\nexit 1\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	err := newCLI("docker", bin, dir).stream(context.Background(), nil, nil, "compose", "-f", "x.yml", "-p", "x", "up", "-d")
	if err == nil || !strings.Contains(err.Error(), "compose up: exit status 1") ||
		!strings.Contains(err.Error(), "bind source path does not exist: /Users/me/missing") {
		t.Fatalf("got %v", err)
	}
}

func TestOutputTail(t *testing.T) {
	tail := &outputTail{}
	for i := range 20 {
		fmt.Fprintf(tail, "line %d\n", i)
	}
	tail.Write([]byte("no newline"))
	got := tail.indented()
	if strings.Contains(got, "line 5\n") || !strings.Contains(got, "line 19") || !strings.HasSuffix(got, "no newline") ||
		strings.Count(got, "\n") != tailLines {
		t.Fatalf("%q", got)
	}
	if (&outputTail{}).indented() != "" {
		t.Fatal("nothing said: nothing added")
	}
}

// Networks reads, in one inspect, whether each takes an address range and its Compose project.
func TestNetworks(t *testing.T) {
	if goruntime.GOOS == "windows" {
		t.Skip("a shell script stands in for docker")
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "docker")
	script := `#!/bin/sh
case "$2" in
ls) echo n1; echo n2 ;;
inspect) echo '[{"Name":"jailwick-api_sandbox","IPAM":{"Config":[{"Subnet":"172.20.0.0/16"}]},"Labels":{"com.docker.compose.project":"jailwick-api"}},{"Name":"host","IPAM":{"Config":[]},"Labels":{}}]' ;;
esac
`
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	nets, err := newCLI("docker", bin, dir).Networks(context.Background())
	want := []Network{{Name: "jailwick-api_sandbox", Addressed: true, Project: "jailwick-api"}, {Name: "host"}}
	if err != nil || !slices.Equal(nets, want) {
		t.Fatalf("%+v %v", nets, err)
	}
}
