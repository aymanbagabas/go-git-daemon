package daemon

import (
	"os"
	"path/filepath"
)

// Returns true if path is a directory containing an `objects` directory and a
// `HEAD` file.
func isGitDir(path string) bool {
	stat, err := os.Stat(filepath.Join(path, "objects"))
	if err != nil {
		return false
	}
	if !stat.IsDir() {
		return false
	}

	stat, err = os.Stat(filepath.Join(path, "HEAD"))
	if err != nil {
		return false
	}
	if stat.IsDir() {
		return false
	}

	return true
}

// isExportOk returns true if path contains a `git-daemon-export-ok` file.
func isExportOk(path string) bool {
	stat, err := os.Stat(filepath.Join(path, "git-daemon-export-ok"))
	if err != nil {
		return false
	}
	if stat.IsDir() {
		return false
	}

	return true
}
