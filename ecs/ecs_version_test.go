package ecs

import (
	"regexp"
	"testing"
)

// semverRE matches MAJOR.MINOR.PATCH with optional pre-release and build metadata.
var semverRE = regexp.MustCompile(`^\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$`)

func TestVersionIsTrimmedSemver(t *testing.T) {
	v := Version()
	if v == "" {
		t.Fatal("Version() is empty")
	}
	if !semverRE.MatchString(v) {
		t.Fatalf("Version() = %q, not a semantic version", v)
	}
}
