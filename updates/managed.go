package updates

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// InstallMethod describes how the application was installed.
type InstallMethod string

const (
	// InstallDirect means the binary was installed directly (download, go install, etc.).
	InstallDirect InstallMethod = ""
	// InstallHomebrew means the binary was installed via Homebrew Cask.
	InstallHomebrew InstallMethod = "homebrew"
)

// homebrewPrefixes are the known Homebrew Caskroom paths.
var homebrewPrefixes = []string{
	"/usr/local/Caskroom/",
	"/opt/homebrew/Caskroom/",
}

// DetectInstallMethod determines how the running binary was installed
// by inspecting its resolved file path.
func DetectInstallMethod() InstallMethod {
	exe, err := os.Executable()
	if err != nil {
		return InstallDirect
	}

	// Resolve symlinks to get the real path
	resolved, err := filepath.EvalSymlinks(exe)
	if err != nil {
		resolved = exe
	}

	if runtime.GOOS == "darwin" {
		for _, prefix := range homebrewPrefixes {
			if strings.HasPrefix(resolved, prefix) {
				return InstallHomebrew
			}
		}
	}

	return InstallDirect
}

// UpdateInstructions returns a user-facing string explaining how to update
// for the given install method.
func (m InstallMethod) UpdateInstructions(appName string) string {
	switch m {
	case InstallHomebrew:
		return "This app was installed via Homebrew. Run: brew upgrade --cask " + appName
	default:
		return ""
	}
}
