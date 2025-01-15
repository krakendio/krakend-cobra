package plugin

import (
	"bufio"
	"io"
	"runtime/debug"
	"sort"
	"strings"

	"github.com/luraproject/lura/v2/core"
	"golang.org/x/mod/semver"
)

// Descriptor lists all the deps and versions required by a binary/plugin
type Descriptor struct {
	Go   string
	Libc string
	Deps map[string]string
}

// Local returns a descriptor for the binary calling it
func Local() Descriptor {
	return Descriptor{
		Go:   core.GoVersion,
		Libc: core.GlibcVersion,
		Deps: getBuildInfo(),
	}
}

// Diff points an incompatibility between descriptors
type Diff struct {
	Name     string
	Expected string
	Have     string
}

// Compare generates a list of diffs (incompatibility) between two descriptors
func (d Descriptor) Compare(other Descriptor) []Diff {
	var diffs []Diff

	for pkgName, expectedVersion := range d.Deps {
		v, ok := other.Deps[pkgName]
		if !ok {
			continue
		}
		if v != expectedVersion {
			diffs = append(diffs, Diff{Name: pkgName, Expected: expectedVersion, Have: v})
		}
	}

	sort.Slice(diffs, func(i, j int) bool { return diffs[i].Name < diffs[j].Name })

	if d.Go != other.Go {
		diffs = prependDiff(diffs, Diff{Name: "go", Expected: d.Go, Have: other.Go})
	}

	if d.Libc != other.Libc {
		diffs = prependDiff(diffs, Diff{Name: "libc", Expected: d.Libc, Have: other.Libc})
	}

	return diffs
}

func prependDiff(diffs []Diff, diff Diff) []Diff {
	tmp := make([]Diff, len(diffs)+1)
	copy(tmp[1:], diffs)
	tmp[0] = diff
	return tmp
}

// Describe reads a go.sum and returns a descriptor for the build or an error
func Describe(r io.Reader, goVersion, libcVersion string) (Descriptor, error) {
	content, err := parseSumFile(r)
	if err != nil {
		return Descriptor{}, err
	}

	deps := map[string]string{}
	for _, dep := range content {
		parts := strings.Split(dep, " ")
		if len(parts) < 2 {
			continue
		}
		cleanedVersion := cleanVersion(parts[1])

		if semver.Compare(deps[parts[0]], cleanedVersion) >= 0 {
			continue
		}
		deps[parts[0]] = cleanedVersion
	}
	return Descriptor{
		Go:   goVersion,
		Deps: deps,
		Libc: libcVersion,
	}, nil
}

func cleanVersion(v string) string {
	if l := len(v); l > 7 && v[l-7:] == "/go.mod" {
		return v[:l-7]
	}
	return v
}

func parseSumFile(r io.Reader) ([]string, error) {
	var lines []string

	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines, scanner.Err()
}

// readBuildInfo is a var to allow the replacement of the function in the tests
// as the debug.ReadBuildInfo doesn't return Deps in tests.
var readBuildInfo = debug.ReadBuildInfo

func getBuildInfo() map[string]string {
	res := map[string]string{}
	bi, ok := readBuildInfo()
	if !ok {
		return res
	}

	for _, dep := range bi.Deps {
		if dep.Replace != nil {
			res[dep.Replace.Path] = dep.Replace.Version
			continue
		}
		res[dep.Path] = dep.Version
	}
	return res
}
