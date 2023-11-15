package tools

import (
	"path"
	"strconv"
	"strings"
)

func importPathToNameBasic(importPath string) (packageName string) {
	base := path.Base(importPath)
	if strings.HasPrefix(base, "v") {
		if _, err := strconv.Atoi(base[1:]); err == nil {
			dir := path.Dir(importPath)
			if dir != "." {
				base = path.Base(dir)
			}
		}
	}
	for _, sep := range []string{".", "-"} {
		parts := strings.Split(base, sep)
		if len(parts) == 1 {
			continue
		}
		if parts[0] == "go" {
			base = parts[1]
			continue
		}
		if parts[1] == "go" {
			base = parts[0]
			continue
		}
		if len(parts[0]) > len(parts[1]) {
			base = parts[0]
			continue
		}
		base = parts[1]
	}
	return base
}
