package runtime

import (
	"context"
	"errors"
	"io"

	"github.com/jailwick/jailwick/internal/spec"
)

// errAppleNotReady is returned until the Apple container driver can keep jailwick's guarantees
// (see docs/RUNTIMES.md): containers there can't be given the host names the sandbox uses, and
// an --internal network still reaches the host.
var errAppleNotReady = errors.New("the apple runtime isn't supported yet (see docs/RUNTIMES.md); use docker (OrbStack, Docker Desktop, Colima) or containerd")

// apple drives Apple's `container` CLI (macOS 26+). It has no Compose, so the driver will create
// networks, volumes and containers itself, in dependency order.
type apple struct{ stateDir string }

func newApple(stateDir string) *apple { return &apple{stateDir: stateDir} }

func (a *apple) Name() string                { return "apple" }
func (a *apple) Check(context.Context) error { return errAppleNotReady }
func (a *apple) Info(context.Context) Info   { return Info{Name: "apple"} }
func (a *apple) Up(context.Context, *spec.Sandbox, UpOptions) error {
	return errAppleNotReady
}
func (a *apple) Down(context.Context, string, bool) error { return errAppleNotReady }
func (a *apple) Containers(context.Context, string) ([]Container, error) {
	return nil, errAppleNotReady
}
func (a *apple) Sandboxes(context.Context) ([]string, error) { return nil, errAppleNotReady }
func (a *apple) AllContainers(context.Context) ([]Container, error) {
	return nil, errAppleNotReady
}
func (a *apple) Restart(context.Context, string) error    { return errAppleNotReady }
func (a *apple) Stop(context.Context, string) error       { return errAppleNotReady }
func (a *apple) Unpause(context.Context, ...string) error { return errAppleNotReady }
func (a *apple) Exec(context.Context, ExecOptions) error  { return errAppleNotReady }
func (a *apple) Logs(context.Context, string, string, LogOptions) error {
	return errAppleNotReady
}
func (a *apple) Stats(context.Context, []string, bool, io.Writer) error { return errAppleNotReady }
func (a *apple) Build(context.Context, BuildOptions) error              { return errAppleNotReady }
func (a *apple) ImageID(context.Context, string) (string, error)        { return "", errAppleNotReady }
func (a *apple) ImageLabels(context.Context, string) (map[string]string, error) {
	return nil, errAppleNotReady
}
func (a *apple) ImagesNamed(context.Context, []string) ([]Image, error) { return nil, nil }
func (a *apple) ImagesWithLabel(context.Context, string) ([]Image, error) {
	return nil, errAppleNotReady
}
func (a *apple) RemoveImage(context.Context, string) error         { return errAppleNotReady }
func (a *apple) ContainerImages(context.Context) ([]string, error) { return nil, errAppleNotReady }
func (a *apple) Networks(context.Context) ([]Network, error)       { return nil, errAppleNotReady }
func (a *apple) RemoveNetwork(context.Context, string) error       { return errAppleNotReady }
