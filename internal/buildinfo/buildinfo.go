// Package buildinfo says which jailwick this is: its version (the VERSION file) and the commit it
// was built from (Go's VCS stamp), in words a person can read.
package buildinfo

import (
	"runtime/debug"
	"strings"
	"time"

	"github.com/jailwick/jailwick"
)

// Version is jailwick's version, e.g. 0.2.5.
var Version = strings.TrimSpace(jailwick.VersionFile)

// Info is a build: its version, and the commit and date it was built from when known.
type Info struct {
	Version  string `json:"version"`
	Commit   string `json:"commit,omitempty"` // short commit hash; "" without a VCS stamp
	Date     string `json:"date,omitempty"`   // the commit's date, 2006-01-02
	Modified bool   `json:"modified,omitempty"`
}

// Get describes this build.
func Get() Info {
	i := Info{Version: Version}
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return i
	}
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			i.Commit = s.Value[:min(len(s.Value), 7)]
		case "vcs.time":
			if t, err := time.Parse(time.RFC3339, s.Value); err == nil {
				i.Date = t.Format(time.DateOnly)
			}
		case "vcs.modified":
			i.Modified = s.Value == "true"
		}
	}
	return i
}

// String is the build as people read it: "0.2.5 (c53f880, 15 Oct 2026)", "0.2.5 (c53f880 with
// changes, …)", or just "0.2.5" without a VCS stamp.
func (i Info) String() string {
	if i.Commit == "" {
		return i.Version
	}
	commit := i.Commit
	if i.Modified {
		commit += " with changes"
	}
	if t, err := time.Parse(time.DateOnly, i.Date); err == nil {
		return i.Version + " (" + commit + ", " + t.Format("2 Jan 2006") + ")"
	}
	return i.Version + " (" + commit + ")"
}
