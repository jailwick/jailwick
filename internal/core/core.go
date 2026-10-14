// Package core opens what every front end (CLI, desktop app, local API) needs: the
// configuration store, the global settings, the secret store and a sandbox manager per runtime.
package core

import (
	"context"
	"sync"

	"github.com/jailwick/jailwick/internal/config"
	"github.com/jailwick/jailwick/internal/paths"
	"github.com/jailwick/jailwick/internal/runtime"
	"github.com/jailwick/jailwick/internal/sandbox"
	"github.com/jailwick/jailwick/internal/secrets"
)

// Env is opened once per process; its parts are created on first use. Safe for concurrent use.
type Env struct {
	Store *config.Store
	// Log receives progress lines from sandbox operations (may be nil).
	Log func(format string, a ...any)
	// Out receives runtime output (builds, compose); nil discards it.
	Out interface{ Write([]byte) (int, error) }

	mu      sync.Mutex
	global  *config.Global
	secrets secrets.Store
	// mgrMu guards managers. It is held while a runtime is checked (seconds, at worst), so
	// callers that only need the settings or secrets don't wait for it.
	mgrMu    sync.Mutex
	managers map[string]*sandbox.Manager // by runtime name
}

// Open returns an environment for the configured folders.
func Open() *Env { return &Env{Store: config.Open()} }

// Global returns config.yaml (loaded once).
func (e *Env) Global() (config.Global, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.global == nil {
		g, err := e.Store.Global()
		if err != nil {
			return g, err
		}
		e.global = &g
	}
	return *e.global, nil
}

// Reload forgets cached settings (after config.yaml changed).
func (e *Env) Reload() {
	e.mgrMu.Lock()
	e.mu.Lock()
	e.global, e.secrets, e.managers = nil, nil, nil
	e.mu.Unlock()
	e.mgrMu.Unlock()
}

// Secrets returns the configured secret store.
func (e *Env) Secrets() (secrets.Store, error) {
	g, err := e.Global()
	if err != nil {
		return nil, err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.secrets == nil {
		s, err := secrets.Open(g.Secrets, e.Store.Dir)
		if err != nil {
			return nil, err
		}
		e.secrets = s
	}
	return e.secrets, nil
}

// RuntimeName is the runtime a project uses (its own setting, else the global one).
func (e *Env) RuntimeName(project string) string {
	g, _ := e.Global()
	if project != "" {
		if p, err := e.Store.Project(project); err == nil && p.Runtime != "" {
			return p.Runtime
		}
	}
	return g.Runtime
}

// Manager returns the sandbox manager for a project's runtime ("" = the global runtime),
// checking once that the runtime is up.
func (e *Env) Manager(ctx context.Context, project string) (*sandbox.Manager, error) {
	g, err := e.Global()
	if err != nil {
		return nil, err
	}
	s, err := e.Secrets()
	if err != nil {
		return nil, err
	}
	name := e.RuntimeName(project)
	e.mgrMu.Lock()
	defer e.mgrMu.Unlock()
	if m, ok := e.managers[name]; ok {
		return m, nil
	}
	d, err := runtime.Open(name, paths.StateDir())
	if err != nil {
		return nil, err
	}
	if err := d.Check(ctx); err != nil {
		return nil, err
	}
	m := &sandbox.Manager{Store: e.Store, Global: g, Secrets: s, Driver: d, StateDir: paths.StateDir(), Log: e.Log}
	if e.Out != nil {
		m.Out = e.Out
	}
	if e.managers == nil {
		e.managers = map[string]*sandbox.Manager{}
	}
	e.managers[name] = m
	return m, nil
}
