package crawlspace

import (
	"runtime/debug"
)

const packageName = "github.com/jtolio/crawlspace"

var crawlspaceVersion string
var processVersion string

func init() {
	crawlspaceVersion = packageName + "@v0.0.0-unknown"
	processVersion = "main@(devel)"
	if bi, ok := debug.ReadBuildInfo(); ok {
		if bi.Main.Path != "" || bi.Main.Version != "" {
			processVersion = bi.Main.Path + "@" + bi.Main.Version
		}
		for _, mod := range bi.Deps {
			if mod.Path == packageName {
				crawlspaceVersion = packageName + "@" + mod.Version
			}
		}
	}
}
