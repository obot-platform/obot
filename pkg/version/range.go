package version

import (
	"fmt"
	"regexp"
	"strings"

	"golang.org/x/mod/semver"
)

const (
	// devVersionPrefix identifies development builds, which are not tagged with a release version.
	devVersionPrefix = "v0.0.0"
)

var (
	// boundPattern matches a full release version with a leading "v", such as v1.2.3. Shorthand
	// versions such as "v0.21" and pre-release or build suffixes are not accepted as bounds.
	boundPattern = regexp.MustCompile(`^v\d+\.\d+\.\d+$`)
)

// IsDev reports whether tag identifies a development build.
func IsDev(tag string) bool {
	return tag == "" || strings.HasPrefix(tag, devVersionPrefix)
}

// ValidateRange checks that minVersion and maxVersion are full release versions (MAJOR.MINOR.PATCH)
// and that minVersion is not greater than maxVersion. Empty values are allowed.
func ValidateRange(minVersion, maxVersion string) error {
	lower, err := normalizeBound(minVersion)
	if err != nil {
		return fmt.Errorf("invalid minObotVersion: %w", err)
	}
	upper, err := normalizeBound(maxVersion)
	if err != nil {
		return fmt.Errorf("invalid maxObotVersion: %w", err)
	}
	if lower != "" && upper != "" && semver.Compare(lower, upper) > 0 {
		return fmt.Errorf("minObotVersion %q must not be greater than maxObotVersion %q", minVersion, maxVersion)
	}
	return nil
}

// InRange reports whether the Obot version tag falls within the inclusive range
// [minVersion, maxVersion]. Empty bounds are unrestricted. Development builds and
// invalid bounds are always in range. Pre-release and build metadata on tag are
// ignored, so a release candidate is treated as the release it precedes.
func InRange(tag, minVersion, maxVersion string) bool {
	if IsDev(tag) || ValidateRange(minVersion, maxVersion) != nil {
		return true
	}

	current, err := normalize(tag)
	if err != nil {
		return true
	}
	current = strings.TrimSuffix(current, semver.Prerelease(current))

	lower, _ := normalizeBound(minVersion)
	upper, _ := normalizeBound(maxVersion)
	return (lower == "" || semver.Compare(current, lower) >= 0) &&
		(upper == "" || semver.Compare(current, upper) <= 0)
}

// normalizeBound returns the canonical form of a range bound, which must be a full release version.
func normalizeBound(v string) (string, error) {
	if v = strings.TrimSpace(v); v != "" && !boundPattern.MatchString(v) {
		return "", fmt.Errorf("%q must be a full release version such as v1.2.3", v)
	}
	return normalize(v)
}

// normalize returns the canonical form of v, which must have a leading "v".
func normalize(v string) (string, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return "", nil
	}
	if !semver.IsValid(v) {
		return "", fmt.Errorf("%q is not a valid semantic version", v)
	}
	return semver.Canonical(v), nil
}

// CurrentInRange reports whether the running Obot version falls within the inclusive range
// [minVersion, maxVersion]. See InRange.
func CurrentInRange(minVersion, maxVersion string) bool {
	return InRange(Tag, minVersion, maxVersion)
}
