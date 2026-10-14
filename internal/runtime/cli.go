package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jailwick/jailwick/internal/compose"
	"github.com/jailwick/jailwick/internal/spec"
)

// cli drives Docker-compatible command lines: docker (OrbStack, Docker Desktop, Colima,
// Docker Engine) and nerdctl (containerd). Both take the same commands and Compose files.
type cli struct {
	name     string
	bin      string
	stateDir string

	mu   sync.Mutex
	info *Info // from the first `info` that answered: it is slow and doesn't change

	live live // kept current by Watch (events.go)
}

func newCLI(name, bin, stateDir string) *cli { return &cli{name: name, bin: bin, stateDir: stateDir} }

func (d *cli) Name() string { return d.name }

// run executes the runtime and returns stdout; stderr is included in the error.
func (d *cli) run(ctx context.Context, env map[string]string, args ...string) ([]byte, error) {
	cmd := d.command(ctx, env, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := d.do(cmd)
	out := stdout.Bytes()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return out, fmt.Errorf("%s %s: %s", d.bin, args[0], msg)
	}
	return out, nil
}

// stream executes the runtime with output going to w. A failure says what the runtime said last
// (its error is there), even when w doesn't keep the output.
func (d *cli) stream(ctx context.Context, env map[string]string, w io.Writer, args ...string) error {
	cmd := d.command(ctx, env, args...)
	tail := &outputTail{}
	out := io.Writer(tail)
	if w != nil {
		out = io.MultiWriter(w, tail)
	}
	cmd.Stdout, cmd.Stderr = out, out
	if err := d.do(cmd); err != nil {
		return fmt.Errorf("%s %s: %w%s", d.bin, TraceKind(cmd.Args), err, tail.indented())
	}
	return nil
}

// outputTail keeps the last lines of a command's output, each cut to a length.
type outputTail struct {
	lines []string
	part  []byte
}

// Output tail bounds: how many lines are kept, and how long each may be.
const (
	tailLines   = 15
	tailLineMax = 300
)

func (t *outputTail) Write(p []byte) (int, error) {
	t.part = append(t.part, p...)
	for {
		i := bytes.IndexByte(t.part, '\n')
		if i < 0 {
			break
		}
		t.keep(string(t.part[:i]))
		t.part = t.part[i+1:]
	}
	if len(t.part) > tailLineMax {
		t.keep(string(t.part))
		t.part = nil
	}
	return len(p), nil
}

func (t *outputTail) keep(line string) {
	line = strings.TrimRight(line, "\r ")
	if line == "" {
		return
	}
	if len(line) > tailLineMax {
		line = line[:tailLineMax] + "…"
	}
	t.lines = append(t.lines, line)
	if len(t.lines) > tailLines {
		t.lines = t.lines[len(t.lines)-tailLines:]
	}
}

// indented is the kept lines, each on a line of its own, indented ("" when there were none).
func (t *outputTail) indented() string {
	if len(t.part) > 0 {
		t.keep(string(t.part))
		t.part = nil
	}
	if len(t.lines) == 0 {
		return ""
	}
	return "\n  " + strings.Join(t.lines, "\n  ")
}

// command is the runtime's command line, with env added to the process's environment. The Docker
// CLI's "What's next" hints are off: they'd follow every session that ends.
func (d *cli) command(ctx context.Context, env map[string]string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, d.bin, args...)
	cmd.Env = withEnv(env)
	return cmd
}

func withEnv(env map[string]string) []string {
	out := append(os.Environ(), "DOCKER_CLI_HINTS=false")
	for _, k := range slices.Sorted(maps.Keys(env)) {
		out = append(out, k+"="+env[k])
	}
	return out
}

// Check runs `info` (kept for Info) and `compose version` side by side.
func (d *cli) Check(ctx context.Context) error {
	if _, err := exec.LookPath(d.bin); err != nil {
		return fmt.Errorf("%s not found on PATH", d.bin)
	}
	var composeErr error
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, composeErr = d.run(ctx, nil, "compose", "version")
	}()
	_, err := d.loadInfo(ctx)
	<-done
	if err != nil {
		return fmt.Errorf("%s is installed but not running (start OrbStack, Docker Desktop or Colima): %w", d.bin, err)
	}
	if composeErr != nil {
		return fmt.Errorf("%s compose is missing: %w", d.bin, composeErr)
	}
	return nil
}

func (d *cli) Info(ctx context.Context) Info {
	info, err := d.loadInfo(ctx)
	if err != nil {
		return Info{Name: d.name}
	}
	return info
}

// loadInfo asks the runtime once; a failed answer isn't kept, so the next call asks again.
func (d *cli) loadInfo(ctx context.Context) (Info, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.info != nil {
		return *d.info, nil
	}
	out, err := d.run(ctx, nil, "info", "--format", "{{json .}}")
	if err != nil {
		return Info{}, err
	}
	var v struct {
		ServerVersion   string
		OperatingSystem string
		Architecture    string
	}
	_ = json.Unmarshal(out, &v)
	info := Info{Name: d.name, Version: v.ServerVersion, Platform: v.OperatingSystem}
	switch v.Architecture {
	case "aarch64", "arm64":
		info.Arch = "arm64"
	case "x86_64", "amd64":
		info.Arch = "amd64"
	}
	if strings.Contains(v.OperatingSystem, "OrbStack") {
		info.HostSuffix = ".orb.local"
	}
	d.info = &info
	return info, nil
}

// composeFile is where a sandbox's Compose file is written: projects/<p>/compose.yml, and
// projects/<p>/sessions/<id>.yml for a session.
func (d *cli) composeFile(sandbox string) string {
	if p, id, ok := spec.ParseSessionName(sandbox); ok {
		return filepath.Join(d.stateDir, "projects", p, "sessions", id+".yml")
	}
	return filepath.Join(d.stateDir, "projects", strings.TrimPrefix(sandbox, spec.Prefix), "compose.yml")
}

func (d *cli) Up(ctx context.Context, sb *spec.Sandbox, opts UpOptions) error {
	data, procEnv, err := compose.Render(sb)
	if err != nil {
		return err
	}
	file := d.composeFile(sb.Name)
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(file, data, 0o644); err != nil {
		return err
	}
	for _, v := range sb.AppVolumes {
		if err := d.ensureVolume(ctx, v); err != nil {
			return err
		}
	}
	maps.Copy(procEnv, opts.Env)
	return d.stream(ctx, procEnv, opts.Out, upArgs(file, sb.Name, opts)...)
}

// upArgs is the compose command that starts a sandbox: all its services (recreating those whose
// definition changed only with Recreate), or just the Only ones, recreated without their
// dependencies.
func upArgs(file, sandbox string, opts UpOptions) []string {
	args := []string{"compose", "-f", file, "-p", sandbox, "up", "-d", "--quiet-pull"}
	switch {
	case len(opts.Only) > 0:
		args = append(args, "--force-recreate", "--no-deps")
	case !opts.Recreate:
		args = append(args, "--no-recreate")
	}
	if opts.RemoveOrphans {
		args = append(args, "--remove-orphans")
	}
	return append(args, opts.Only...)
}

// ensureVolume creates an app volume unless it exists. Compose only references app volumes
// (external), so it never creates or removes them.
func (d *cli) ensureVolume(ctx context.Context, name string) error {
	if _, err := d.run(ctx, nil, "volume", "inspect", name); err == nil {
		return nil
	}
	if _, err := d.run(ctx, nil, "volume", "create", name); err != nil {
		// Another jw may have created it meanwhile.
		if _, again := d.run(ctx, nil, "volume", "inspect", name); again == nil {
			return nil
		}
		return err
	}
	return nil
}

func (d *cli) Down(ctx context.Context, sandbox string, volumes bool) error {
	args := []string{"compose", "-p", sandbox}
	if f := d.composeFile(sandbox); exists(f) {
		args = append(args, "-f", f)
	}
	args = append(args, "down", "--remove-orphans")
	if volumes {
		args = append(args, "--volumes")
	}
	// Compose interpolates secrets even for down; empty values are fine here.
	_, err := d.run(ctx, blankSecrets(d.composeFile(sandbox)), args...)
	if _, _, ok := spec.ParseSessionName(sandbox); ok && err == nil {
		_ = os.Remove(d.composeFile(sandbox)) // a session's file isn't needed again
	}
	return err
}

// blankSecrets returns empty values for the secret variables a compose file references,
// so commands that don't start containers don't warn about unset variables.
func blankSecrets(file string) map[string]string {
	data, err := os.ReadFile(file)
	if err != nil {
		return nil
	}
	env := map[string]string{}
	for _, part := range strings.Split(string(data), "${")[1:] {
		if name, _, ok := strings.Cut(part, "}"); ok && strings.HasPrefix(name, "JAILWICK_SECRET_ENV_") {
			env[name] = ""
		}
	}
	return env
}

func (d *cli) inspect(ctx context.Context, ids []string) ([]Container, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	out, err := d.run(ctx, nil, append([]string{"inspect"}, ids...)...)
	if err != nil {
		return nil, err
	}
	var raw []struct {
		Name         string
		Image        string
		Created      time.Time
		RestartCount int
		State        struct {
			Status   string
			ExitCode int
			Health   *struct{ Status string }
		}
		Config     struct{ Labels map[string]string }
		HostConfig struct{ RestartPolicy struct{ Name string } }
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("%s inspect: %w", d.bin, err)
	}
	cs := make([]Container, 0, len(raw))
	for _, r := range raw {
		c := Container{
			Name: strings.TrimPrefix(r.Name, "/"), State: r.State.Status,
			ImageID: r.Image, Labels: r.Config.Labels, Created: r.Created,
			Service:  r.Config.Labels["com.docker.compose.service"],
			Restarts: r.RestartCount, ExitCode: r.State.ExitCode, Policy: r.HostConfig.RestartPolicy.Name,
		}
		if r.State.Health != nil {
			c.Health = r.State.Health.Status
		}
		cs = append(cs, c)
	}
	slices.SortFunc(cs, func(a, b Container) int { return strings.Compare(a.Service, b.Service) })
	return cs, nil
}

func (d *cli) ids(ctx context.Context, filter string) ([]string, error) {
	out, err := d.run(ctx, nil, "ps", "-a", "-q", "--filter", "label="+filter)
	if err != nil {
		return nil, err
	}
	return strings.Fields(string(out)), nil
}

func (d *cli) Containers(ctx context.Context, sandbox string) ([]Container, error) {
	key, value := sandboxLabel(sandbox)
	if cs, ok := d.cached(); ok {
		return slices.DeleteFunc(cs, func(c Container) bool { return c.Labels[key] != value }), nil
	}
	ids, err := d.ids(ctx, key+"="+value)
	if err != nil {
		return nil, err
	}
	return d.inspect(ctx, ids)
}

func (d *cli) Restart(ctx context.Context, container string) error {
	_, err := d.run(ctx, nil, "restart", "-t", "2", container)
	return err
}

func (d *cli) Stop(ctx context.Context, container string) error {
	_, err := d.run(ctx, nil, "stop", "-t", "5", container)
	return err
}

func (d *cli) Unpause(ctx context.Context, containers ...string) error {
	if len(containers) == 0 {
		return nil
	}
	_, err := d.run(ctx, nil, append([]string{"unpause"}, containers...)...)
	return err
}

func (d *cli) Sandboxes(ctx context.Context) ([]string, error) {
	cs, err := d.AllContainers(ctx)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, c := range cs {
		if p := c.Labels[spec.LabelProject]; p != "" && !slices.Contains(out, p) {
			out = append(out, p)
		}
	}
	slices.Sort(out)
	return out, nil
}

// sandboxLabel is the label a sandbox's containers carry: a session's id, or the project's name
// (which a project's session containers carry too).
func sandboxLabel(sandbox string) (key, value string) {
	if _, id, ok := spec.ParseSessionName(sandbox); ok {
		return spec.LabelSession, id
	}
	return spec.LabelProject, strings.TrimPrefix(sandbox, spec.Prefix)
}

func (d *cli) AllContainers(ctx context.Context) ([]Container, error) {
	if cs, ok := d.cached(); ok {
		return cs, nil
	}
	return d.listAll(ctx)
}

// listAll asks the runtime for every jailwick container.
func (d *cli) listAll(ctx context.Context) ([]Container, error) {
	ids, err := d.ids(ctx, spec.LabelProject)
	if err != nil {
		return nil, err
	}
	return d.inspect(ctx, ids)
}

func (d *cli) Exec(ctx context.Context, o ExecOptions) error {
	args := []string{"exec"}
	if o.TTY || o.Terminal != nil {
		args = append(args, "-it")
	} else if o.Stdin != nil {
		args = append(args, "-i")
	}
	if o.User != "" {
		args = append(args, "-u", o.User)
	}
	if o.WorkDir != "" {
		args = append(args, "-w", o.WorkDir)
	}
	for _, k := range slices.Sorted(maps.Keys(o.Env)) {
		args = append(args, "-e", k+"="+o.Env[k])
	}
	args = append(args, o.Container)
	args = append(args, o.Cmd...)
	cmd := d.command(ctx, nil, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = o.Stdin, o.Stdout, o.Stderr
	if o.TTY && cmd.Stdin == nil {
		cmd.Stdin = os.Stdin
	}
	if o.Terminal != nil {
		cmd.Stdin, cmd.Stdout, cmd.Stderr = o.Terminal, o.Terminal, o.Terminal
		controlledBy(cmd) // the runtime's client follows the terminal's size
	}
	err := d.do(cmd)
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return &ExitError{Code: ee.ExitCode()}
	}
	return err
}

// ExitError carries the exit status of a command run in a sandbox.
type ExitError struct{ Code int }

func (e *ExitError) Error() string { return "exit status " + strconv.Itoa(e.Code) }

func (d *cli) Logs(ctx context.Context, sandbox, service string, o LogOptions) error {
	var args []string
	if service == "" {
		args = []string{"compose", "-p", sandbox, "-f", d.composeFile(sandbox), "logs"}
	} else {
		args = []string{"logs"}
	}
	if o.Follow {
		args = append(args, "-f")
	}
	if o.Tail > 0 {
		args = append(args, "--tail", strconv.Itoa(o.Tail))
	}
	if !o.Since.IsZero() {
		args = append(args, "--since", o.Since.UTC().Format(time.RFC3339Nano))
	}
	if o.Timestamps {
		args = append(args, "--timestamps")
	}
	if service != "" {
		args = append(args, ContainerName(sandbox, service))
	}
	return d.stream(ctx, blankSecrets(d.composeFile(sandbox)), o.Out, args...)
}

func (d *cli) Stats(ctx context.Context, sandboxes []string, once bool, w io.Writer) error {
	var names []string
	for _, s := range sandboxes {
		cs, err := d.Containers(ctx, s)
		if err != nil {
			return err
		}
		for _, c := range cs {
			if c.Running() {
				names = append(names, c.Name)
			}
		}
	}
	if len(names) == 0 {
		fmt.Fprintln(w, "No running sandboxes.")
		return nil
	}
	args := []string{"stats", "--format", "table {{.Name}}\t{{.CPUPerc}}\t{{.MemUsage}}\t{{.PIDs}}"}
	if once {
		args = append(args, "--no-stream")
	}
	args = append(args, names...)
	cmd := d.command(ctx, nil, args...)
	cmd.Stdout, cmd.Stderr = w, w
	return d.do(cmd)
}

// Build runs a BuildKit build (jailwick's Dockerfiles use cache mounts) with plain progress: one
// event per line, which the caller logs and shows.
func (d *cli) Build(ctx context.Context, o BuildOptions) error {
	var env map[string]string
	if d.name == "docker" {
		env = map[string]string{"DOCKER_BUILDKIT": "1"} // a missing buildx fails clearly
	}
	return d.stream(ctx, env, o.Out, buildArgs(o)...)
}

func buildArgs(o BuildOptions) []string {
	args := []string{"build", "--progress=plain", "-t", o.Tag}
	if o.Pull {
		args = append(args, "--pull")
	}
	if o.NoCache {
		args = append(args, "--no-cache")
	}
	if o.Platform != "" {
		args = append(args, "--platform", o.Platform)
	}
	if o.Dockerfile != "" {
		args = append(args, "-f", o.Dockerfile)
	}
	for _, k := range slices.Sorted(maps.Keys(o.Args)) {
		args = append(args, "--build-arg", k+"="+o.Args[k])
	}
	for _, k := range slices.Sorted(maps.Keys(o.Labels)) {
		args = append(args, "--label", k+"="+o.Labels[k])
	}
	return append(args, o.Context)
}

// Networks lists every network with one inspect: whether it takes an address range, and its
// Compose project.
func (d *cli) Networks(ctx context.Context) ([]Network, error) {
	out, err := d.run(ctx, nil, "network", "ls", "-q")
	if err != nil {
		return nil, err
	}
	ids := strings.Fields(string(out))
	if len(ids) == 0 {
		return nil, nil
	}
	out, err = d.run(ctx, nil, append([]string{"network", "inspect"}, ids...)...)
	if err != nil {
		return nil, err
	}
	var raw []struct {
		Name   string
		IPAM   struct{ Config []json.RawMessage }
		Labels map[string]string
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("%s network inspect: %w", d.bin, err)
	}
	nets := make([]Network, 0, len(raw))
	for _, r := range raw {
		nets = append(nets, Network{Name: r.Name, Addressed: len(r.IPAM.Config) > 0, Project: r.Labels["com.docker.compose.project"]})
	}
	return nets, nil
}

func (d *cli) RemoveNetwork(ctx context.Context, name string) error {
	_, err := d.run(ctx, nil, "network", "rm", name)
	return err
}

// ImageID asks the runtime, or remembers the answer while the events stream runs (an image event
// forgets it).
func (d *cli) ImageID(ctx context.Context, ref string) (string, error) {
	if id, ok := d.cachedImage(ref); ok {
		return id, nil
	}
	out, err := d.run(ctx, nil, "image", "inspect", "--format", "{{.Id}}", ref)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "no such") || strings.Contains(err.Error(), "not found") {
			d.keepImage(ref, "")
			return "", nil
		}
		return "", err
	}
	id := strings.TrimSpace(string(out))
	d.keepImage(ref, id)
	return id, nil
}

func (d *cli) ImageLabels(ctx context.Context, ref string) (map[string]string, error) {
	out, err := d.run(ctx, nil, "image", "inspect", "--format", "{{json .Config.Labels}}", ref)
	if err != nil {
		return nil, err
	}
	var labels map[string]string
	if err := json.Unmarshal(bytes.TrimSpace(out), &labels); err != nil {
		return nil, err
	}
	return labels, nil
}

func (d *cli) ImagesWithLabel(ctx context.Context, label string) ([]Image, error) {
	out, err := d.run(ctx, nil, "images", "--filter", "label="+label, "--format", "{{json .}}")
	if err != nil {
		return nil, err
	}
	var imgs []Image
	for _, line := range bytes.Split(bytes.TrimSpace(out), []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		var v struct{ Repository, Tag, ID, Size, CreatedSince string }
		if json.Unmarshal(line, &v) == nil {
			imgs = append(imgs, Image{Ref: v.Repository + ":" + v.Tag, ID: v.ID, Size: v.Size, Created: v.CreatedSince})
		}
	}
	return imgs, nil
}

func (d *cli) ImagesNamed(ctx context.Context, refs []string) ([]Image, error) {
	var imgs []Image
	for _, ref := range refs {
		out, err := d.run(ctx, nil, "images", "--format", "{{json .}}", ref)
		if err != nil {
			return nil, err
		}
		for _, line := range bytes.Split(bytes.TrimSpace(out), []byte("\n")) {
			var v struct{ Repository, Tag, ID, Size, CreatedSince string }
			if len(line) > 0 && json.Unmarshal(line, &v) == nil {
				imgs = append(imgs, Image{Ref: v.Repository + ":" + v.Tag, ID: v.ID, Size: v.Size, Created: v.CreatedSince})
			}
		}
	}
	return imgs, nil
}

func (d *cli) RemoveImage(ctx context.Context, ref string) error {
	_, err := d.run(ctx, nil, "rmi", ref)
	return err
}

func (d *cli) ContainerImages(ctx context.Context) ([]string, error) {
	var ids []string
	for _, label := range []string{spec.LabelProject, spec.LabelSession} {
		found, err := d.ids(ctx, label)
		if err != nil {
			return nil, err
		}
		for _, id := range found {
			if !slices.Contains(ids, id) {
				ids = append(ids, id)
			}
		}
	}
	cs, err := d.inspect(ctx, ids)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, c := range cs {
		out = append(out, c.ImageID)
	}
	return out, nil
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
