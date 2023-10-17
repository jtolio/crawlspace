package crawlspace

import (
	"runtime/debug"
)

var crawlspaceVersion = func() string {
	if bi, ok := debug.ReadBuildInfo(); ok {
		for _, mod := range bi.Deps {
			if mod.Path == "github.com/jtolio/crawlspace" {
				return mod.Version
			}
		}
	}
	return "v0.0.0-unknown"
}()
