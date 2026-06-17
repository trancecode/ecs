package ecs

import (
	_ "embed"
	"strings"
)

//go:embed VERSION
var versionFile string

// Version returns the module's semantic version (https://semver.org), read from
// the embedded VERSION file. The VERSION file is the single source of truth;
// keep it in sync with the matching git tag (vMAJOR.MINOR.PATCH).
func Version() string {
	return strings.TrimSpace(versionFile)
}
