// jailwick-app is jailwick's desktop app: a window with the same UI as `jw app --browser`
// (served in-process, no network socket) and a menu bar item with every project.
package main

import (
	"context"
	"encoding/json"
	"log"
	"path/filepath"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"github.com/jailwick/jailwick/internal/buildinfo"
	"github.com/jailwick/jailwick/internal/core"
	"github.com/jailwick/jailwick/internal/paths"
	"github.com/jailwick/jailwick/internal/runtime"
	"github.com/jailwick/jailwick/internal/sandbox"
	"github.com/jailwick/jailwick/internal/server"
	"github.com/jailwick/jailwick/internal/webui"
)

func main() {
	useLoginPath(filepath.Join(paths.StateDir(), "login-path"))
	env := core.Open()
	// Check the runtime while the window loads: the first page then finds it ready.
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_, _ = env.Manager(ctx, "")
	}()
	handler := server.New(env, webui.FS(), "", buildinfo.Version) // in-process: no token needed
	// Notifications are the app's own; a click shows the window at the page about them.
	var show func(route string)
	notifier := newAppNotifier(func(route string) { show(route) })
	handler.Notify = notifier.Send
	// The menu bar looks again when a project may have changed (see trayMenu.follow).
	changed := make(chan struct{}, 1)
	handler.OnChange = func() {
		select {
		case changed <- struct{}{}:
		default: // a look is already asked for: it will see this change too
		}
	}
	go handler.Watch(context.Background()) // notifications, also from the menu bar
	// Quitting (⌘Q, the Dock, the menu bar) asks first while a start, restart or stop runs, and
	// cleans up before it quits.
	guard := &server.QuitGuard{Server: handler}

	app := application.New(application.Options{
		Name:        "jailwick",
		Description: "AI coding agents in a sandbox per repo",
		Icon:        appIconNight,
		Assets:      application.AssetOptions{Handler: handler, DisableLogging: true},
		ShouldQuit:  guard.ShouldQuit,
		Services:    []application.Service{application.NewService(notifier)},
		Mac: application.MacOptions{
			// Closing the window keeps jailwick in the menu bar.
			ApplicationShouldTerminateAfterLastWindowClosed: false,
		},
	})
	guard.Quit = app.Quit

	win := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:     "jailwick",
		Width:     1320,
		Height:    840,
		MinWidth:  980,
		MinHeight: 600,
		URL:       "/",
	})
	// Folder dialogs are the app's own, as sheets on its window.
	handler.PickFolder = func(_ context.Context, title, start string) (string, error) {
		path, err := app.Dialog.OpenFile().
			CanChooseDirectories(true).CanChooseFiles(false).CanCreateDirectories(true).
			SetTitle(title).SetDirectory(start).AttachToWindow(win).
			PromptForSingleSelection()
		if err != nil || path == "" {
			return "", err
		}
		return filepath.Clean(path), nil
	}
	win.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		e.Cancel()
		win.Hide()
	})
	// A route replaces the page's place in history rather than adding one: the app has its own Back.
	// The window shows first, so a hidden window's page is awake to follow it. "" leaves the page
	// where it was.
	show = func(route string) {
		win.Show()
		win.Focus()
		if route != "" {
			hash, _ := json.Marshal("#/" + route)
			win.ExecJS("location.replace(" + string(hash) + ")")
		}
	}

	// The Dock icon follows the system appearance: night when dark, day when light.
	matchAppearance := func(*application.ApplicationEvent) { app.SetIcon(appIcon(app.Env.IsDarkMode())) }
	app.Event.OnApplicationEvent(events.Common.ApplicationStarted, matchAppearance)
	app.Event.OnApplicationEvent(events.Common.ThemeChanged, matchAppearance)

	tray := app.SystemTray.New()
	tray.SetTemplateIcon(trayIcon)
	tray.SetTooltip("jailwick")
	menu := newTrayMenu(app, env, handler, show)
	tray.SetMenu(menu.menu)
	go menu.follow(changed)

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}

// trayMenu lists the projects with their state and quick actions.
type trayMenu struct {
	app     *application.App
	env     *core.Env
	handler *server.Server // runs the actions, tracked like the page's (and cancelled on quit)
	show    func(route string)
	menu    *application.Menu
}

func newTrayMenu(app *application.App, env *core.Env, handler *server.Server, show func(string)) *trayMenu {
	return &trayMenu{app: app, env: env, handler: handler, show: show, menu: app.NewMenu()}
}

// Menu bar timings: a burst of changes (a restart's containers) makes one look, after
// trayPause; with no change it looks every trayEvery anyway (a start from jw shows as it begins,
// before its containers change).
const (
	trayPause = 300 * time.Millisecond
	trayEvery = 10 * time.Second
)

// follow refreshes the menu when told of a change, and every trayEvery.
func (t *trayMenu) follow(changed <-chan struct{}) {
	for {
		t.refresh()
		select {
		case <-changed:
			time.Sleep(trayPause)
		case <-time.After(trayEvery):
		}
	}
}

func (t *trayMenu) refresh() {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	var by map[string][]runtime.Container
	m, err := t.env.Manager(ctx, "")
	if err == nil {
		by, _ = m.ProjectContainers(ctx)
	}
	ps, _ := t.env.Store.Projects()

	t.menu.Clear()
	t.menu.Add("Open jailwick").OnClick(func(*application.Context) { t.show("") })
	t.menu.AddSeparator()
	if err != nil {
		t.menu.Add("Runtime not running").SetEnabled(false)
	}
	for _, p := range ps {
		name, dir := p.Name, p.Dir
		on, health := sandbox.MainRunning(by[name]), sandbox.HealthOf(by[name])
		phase := t.handler.Phase(name)
		if phase == "" && health.Main == "restarting" {
			phase = "Recovering…" // it crashed: the runtime brings it back
		}
		label := "○  " + name
		switch {
		case phase != "":
			label = "◌  " + name
		case health.Main == "paused":
			label = "◐  " + name
		case on && len(health.Problems) > 0:
			label = "⚠  " + name
		case on:
			label = "●  " + name
		}
		sub := t.menu.AddSubmenu(label)
		sub.Add("Show").OnClick(func(*application.Context) { t.show("projects/" + name) })
		for _, p := range health.Problems {
			sub.Add(p.Service + ": " + p.What).OnClick(func(*application.Context) { t.show("projects/" + name + "/logs?service=" + p.Service) })
		}
		if phase != "" {
			sub.Add(phase).SetEnabled(false) // its containers come and go: nothing to offer meanwhile
		} else if health.Main == "paused" {
			sub.Add("Paused").SetEnabled(false)
			sub.Add("Resume").OnClick(func(*application.Context) { t.act(name, server.ActStart) })
			sub.Add("Stop").OnClick(func(*application.Context) { t.act(name, server.ActStop) })
		} else if on {
			sub.Add("New session").OnClick(func(*application.Context) {
				proj, _ := t.env.Store.Project(name)
				g, _ := t.env.Global()
				_ = server.OpenTerminal(g.Terminal, proj, server.Session{})
			})
			sub.Add("Restart").OnClick(func(*application.Context) { t.act(name, server.ActRestart) })
			sub.Add("Stop").OnClick(func(*application.Context) { t.act(name, server.ActStop) })
		} else {
			sub.Add("Start").OnClick(func(*application.Context) { t.act(name, server.ActStart) })
		}
		_ = dir
	}
	t.menu.AddSeparator()
	t.menu.Add("Quit jailwick").OnClick(func(*application.Context) { t.app.Quit() })
	t.menu.Update()
}

// act runs a life-cycle action in the background, tracked like the page's (the page shows it,
// and quitting asks while it runs); its beginning and end refresh the menu (OnChange).
func (t *trayMenu) act(project, action string) {
	go func() {
		if _, err := t.handler.Act(project, action, false); err != nil {
			log.Printf("%s %s: %v", action, project, err)
		}
	}()
}
