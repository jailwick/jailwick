package server

import (
	"net/http"
	"path/filepath"
	goruntime "runtime"

	"github.com/jailwick/jailwick/internal/assets"
	"github.com/jailwick/jailwick/internal/buildinfo"
	"github.com/jailwick/jailwick/internal/paths"
)

// AboutView is what the About page shows: this build, the machine and runtime it works with, and
// where jailwick keeps its files.
type AboutView struct {
	Version  string            `json:"version"`
	Build    buildinfo.Info    `json:"build"`           // the version with the commit and date it was built from
	Agent    *AgentView        `json:"agent,omitempty"` // nil for a build without agent info
	OS       string            `json:"os"`
	Arch     string            `json:"arch"`
	Go       string            `json:"go"`
	Runtime  RuntimeView       `json:"runtime"`
	Folders  map[string]string `json:"folders"`
	Projects int               `json:"projects"`
	Spaces   int               `json:"workspaces"`
}

// AgentView is the build of the agent this jailwick mounts: its label and latest changes.
type AgentView struct {
	Label   string               `json:"label"`
	Changes []assets.AgentChange `json:"changes"`
}

// aboutChanges is how many of the agent's latest changes About lists.
const aboutChanges = 5

func (s *Server) about(w http.ResponseWriter, r *http.Request) {
	rt, _ := s.runtimeView(r.Context())
	ps, _ := s.env.Store.Projects()
	state := paths.StateDir()
	v := AboutView{
		Version: s.version, Build: buildinfo.Get(), OS: goruntime.GOOS, Arch: goruntime.GOARCH, Go: goruntime.Version(), Runtime: rt,
		Projects: len(ps), Spaces: len(s.env.Store.Workspaces()),
		Folders: map[string]string{
			"config":     paths.Tilde(s.env.Store.Dir),
			"state":      paths.Tilde(state),
			"data":       paths.Tilde(paths.DataDir()),
			"build_logs": paths.Tilde(filepath.Join(state, "build", "logs")),
		},
	}
	if info := assets.AgentInfoOf(); info != nil {
		v.Agent = &AgentView{Label: info.Label(), Changes: info.Changes[:min(len(info.Changes), aboutChanges)]}
	}
	writeJSON(w, v)
}
