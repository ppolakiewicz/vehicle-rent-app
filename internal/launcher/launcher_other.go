//go:build !windows

package launcher

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// DataDir is ~/Wypozyczalnia on systems other than Windows (development).
func DataDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Wypozyczalnia"), nil
}

func OpenWindow(url, profileDir string) (*Window, error) {
	for _, exe := range []string{"microsoft-edge", "google-chrome", "chromium", "chromium-browser"} {
		if p, err := exec.LookPath(exe); err == nil {
			if w, err := startTracked(exec.Command(p, appFlags(url, profileDir)...)); err == nil {
				return w, nil
			}
		}
	}
	return &Window{}, exec.Command("xdg-open", url).Start()
}

func OpenFolder(path string) error {
	return exec.Command("xdg-open", path).Start()
}

func ShowError(title, msg string) {
	fmt.Fprintf(os.Stderr, "%s: %s\n", title, msg)
}
