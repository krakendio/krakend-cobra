package plugin

import (
	"runtime/debug"
	"testing"

	"github.com/stretchr/testify/require"
)

func testReadBuildInfo() (info *debug.BuildInfo, ok bool) {
	return &debug.BuildInfo{
		GoVersion: "go1.16.3",
		Deps: []*debug.Module{
			{
				Path:    "github.com/ugorji/go",
				Version: "v1.1.4",
				Replace: &debug.Module{
					Path:    "github.com/ugorji/go/codec",
					Version: "v0.0.0-20190204201341-e444a5086c43",
				},
			},
		},
	}, true
}

func Test_getBuildInfo(t *testing.T) {
	orig := readBuildInfo
	readBuildInfo = testReadBuildInfo
	t.Cleanup(func() {
		readBuildInfo = orig
	})
	got := getBuildInfo()
	val := got["github.com/ugorji/go/codec"]
	require.Equal(t, "v0.0.0-20190204201341-e444a5086c43", val)
}
