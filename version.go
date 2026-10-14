// Package jailwick holds what belongs to the whole repo: its version.
package jailwick

import _ "embed"

// VersionFile is the VERSION file: jailwick's version, the one place it is set. Change it there
// for a release; internal/buildinfo reads it.
//
//go:embed VERSION
var VersionFile string
