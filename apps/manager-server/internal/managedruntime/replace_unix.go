//go:build !windows

package managedruntime

import "os"

func replaceRuntimeFile(source string, target string) error {
	return os.Rename(source, target)
}
