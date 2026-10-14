package server

import "net/http"

// NodeModulesView is which node_modules the sandbox keeps apart from the Mac's: the package
// folders found automatically (Auto) or the project's own list (Setting, folders or patterns),
// as folders (Dirs, the repo's top aside: its own always is), and those with nothing installed
// in the sandbox yet (Empty, "." for the top).
type NodeModulesView struct {
	Auto    bool     `json:"auto"`
	Setting []string `json:"setting"`
	Dirs    []string `json:"dirs"`
	Empty   []string `json:"empty"`
	// Used: the repo has a package.json (at the top or in a package folder), or folders are
	// chosen; otherwise there is nothing to keep apart and the page leaves the card out.
	Used bool `json:"used"`
}

// saveNodeModules sets the project's node_modules folders: {dirs}, empty for automatic. A
// restart applies it (the page lists it as pending).
func (s *Server) saveNodeModules(w http.ResponseWriter, r *http.Request) {
	p, err := s.env.Store.Project(r.PathValue("name"))
	if err != nil {
		apiError(w, err)
		return
	}
	var req struct {
		Dirs []string `json:"dirs"`
	}
	if err := readJSON(r, &req); err != nil {
		apiError(w, err)
		return
	}
	p.NodeModules = nil
	for _, d := range req.Dirs {
		if d != "" {
			p.NodeModules = append(p.NodeModules, d)
		}
	}
	if err := p.Validate(); err != nil {
		apiError(w, err)
		return
	}
	if err := s.env.Store.SaveProject(p); err != nil {
		apiError(w, err)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}
