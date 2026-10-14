package cli

import (
	"context"
	"errors"
	"fmt"
	"github.com/jailwick/jailwick/internal/buildinfo"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/jailwick/jailwick/internal/hostutil"
	"github.com/jailwick/jailwick/internal/paths"
	"github.com/jailwick/jailwick/internal/server"
	"github.com/jailwick/jailwick/internal/webui"
)

func (a *app) appCmd() *cobra.Command {
	var browser, noOpen bool
	var port int
	cmd := &cobra.Command{
		Use:   "app",
		Short: "Open the jailwick app (desktop window, or --browser)",
		Long: `Open jailwick's app: projects, sessions, worktrees, network and logs at a glance.

The desktop app is used when it is installed (macOS: jailwick.app in ~/Applications or
/Applications; elsewhere jailwick-app next to jailwick or on PATH). --browser serves the same app
on 127.0.0.1 with a one-time token instead, and opens it in your browser.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !browser {
				if desktop := desktopApp(); desktop != nil {
					return desktop.Start()
				}
				a.ui.Step("the desktop app isn't installed; opening in your browser")
			}
			return a.serveBrowser(cmd.Context(), port, !noOpen)
		},
	}
	cmd.Flags().BoolVar(&browser, "browser", false, "serve the app to your browser")
	cmd.Flags().IntVar(&port, "port", 0, "port on 127.0.0.1 (default: a free one)")
	cmd.Flags().BoolVar(&noOpen, "no-open", false, "print the link without opening the browser")
	return cmd
}

// desktopApp returns the command that opens the installed desktop app, or nil: on macOS the
// jailwick.app bundle (opened like any app, so a second `jw app` brings it forward), else the
// jailwick-app binary next to jailwick or on PATH.
func desktopApp() *exec.Cmd {
	if goruntime.GOOS == "darwin" {
		for _, dir := range []string{filepath.Join(paths.Home(), "Applications"), "/Applications"} {
			if bundle := filepath.Join(dir, "jailwick.app"); hostutil.Exists(bundle) {
				return exec.Command("open", bundle)
			}
		}
	}
	name := "jailwick-app"
	if goruntime.GOOS == "windows" {
		name += ".exe"
	}
	if exe, err := os.Executable(); err == nil {
		if p := filepath.Join(filepath.Dir(exe), name); hostutil.Exists(p) {
			return exec.Command(p)
		}
	}
	if p, err := exec.LookPath(name); err == nil {
		return exec.Command(p)
	}
	return nil
}

func (a *app) serveBrowser(ctx context.Context, port int, open bool) error {
	ln, err := listenApp(port)
	if err != nil {
		return err
	}
	token := server.NewToken()
	handler := server.New(a.env, webui.FS(), token, buildinfo.Version)
	srv := &http.Server{Handler: handler, ReadHeaderTimeout: 10 * time.Second}
	go handler.Watch(ctx) // notifications while the app runs
	link := fmt.Sprintf("http://127.0.0.1:%d/?token=%s", ln.Addr().(*net.TCPAddr).Port, token)
	a.ui.OK("jailwick app at %s", link)
	a.ui.Hint("only this link works (a one-time token); Ctrl-C stops the app")
	if open {
		_ = hostutil.OpenURL(link)
	}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}()
	if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// appPortFile remembers the port the browser app last served on.
func appPortFile() string { return filepath.Join(paths.StateDir(), "app-port") }

// listenApp listens on port, or without one on the port used last time when it is free (else
// any): the page keeps its origin, so what the browser keeps for it (the theme, the project
// last shown) is still there next time.
func listenApp(port int) (net.Listener, error) {
	if port != 0 {
		return net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	}
	if data, err := os.ReadFile(appPortFile()); err == nil {
		if last, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil && last > 0 {
			if ln, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(last))); err == nil {
				return ln, nil
			}
		}
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	_ = os.MkdirAll(filepath.Dir(appPortFile()), 0o755)
	_ = os.WriteFile(appPortFile(), []byte(strconv.Itoa(ln.Addr().(*net.TCPAddr).Port)+"\n"), 0o644)
	return ln, nil
}
