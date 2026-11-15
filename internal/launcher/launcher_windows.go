//go:build windows

package launcher

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// DataDir is "Dokumenty\Wypożyczalnia" (the real Documents folder, also when
// it is redirected to OneDrive).
func DataDir() (string, error) {
	docs, err := windows.KnownFolderPath(windows.FOLDERID_Documents, 0)
	if err != nil || docs == "" {
		home, herr := os.UserHomeDir()
		if herr != nil {
			return "", herr
		}
		docs = filepath.Join(home, "Documents")
	}
	return filepath.Join(docs, "Wypożyczalnia"), nil
}

func browserCandidates() []string {
	var out []string
	for _, env := range []string{"ProgramFiles(x86)", "ProgramFiles", "LocalAppData"} {
		if base := os.Getenv(env); base != "" {
			out = append(out, filepath.Join(base, `Microsoft\Edge\Application\msedge.exe`))
		}
	}
	for _, env := range []string{"ProgramFiles", "ProgramFiles(x86)", "LocalAppData"} {
		if base := os.Getenv(env); base != "" {
			out = append(out, filepath.Join(base, `Google\Chrome\Application\chrome.exe`))
		}
	}
	return out
}

// OpenWindow opens the application in an Edge (or Chrome) app window, or in
// the default browser as a last resort.
func OpenWindow(url, profileDir string) (*Window, error) {
	for _, exe := range browserCandidates() {
		if !fileExists(exe) {
			continue
		}
		cmd := exec.Command(exe, appFlags(url, profileDir)...)
		if w, err := startTracked(cmd); err == nil {
			return w, nil
		}
	}
	err := exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	return &Window{}, err
}

// OpenFolder shows a folder in Explorer.
func OpenFolder(path string) error {
	os.MkdirAll(path, 0o755)
	return exec.Command("explorer.exe", path).Start()
}

// ShowError shows a message box (the program has no console window).
func ShowError(title, msg string) {
	user32 := syscall.NewLazyDLL("user32.dll")
	box := user32.NewProc("MessageBoxW")
	t, _ := syscall.UTF16PtrFromString(title)
	m, _ := syscall.UTF16PtrFromString(msg)
	const mbIconError = 0x10
	box.Call(0, uintptr(unsafe.Pointer(m)), uintptr(unsafe.Pointer(t)), mbIconError)
}
