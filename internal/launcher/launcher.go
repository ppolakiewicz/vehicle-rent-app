// Package launcher contains the operating-system specific parts: where data is
// kept, opening the application window and showing error messages.
package launcher

import (
	"os"
	"os/exec"
	"time"
)

// Window is an opened application window (browser process).
type Window struct {
	cmd     *exec.Cmd
	started time.Time
	done    chan struct{}
}

// Done is closed when the browser process we started exits. It is nil if the
// window was opened in a way we cannot track (default browser).
func (w *Window) Done() <-chan struct{} {
	if w == nil {
		return nil
	}
	return w.done
}

// Tracked reports whether closing the process means the window was closed:
// a browser that exits within a few seconds handed the window over to an
// already running copy, so we cannot track it.
func (w *Window) Tracked() bool {
	return w != nil && w.cmd != nil
}

// LivedLong reports whether the process ran long enough to have been the real window.
func (w *Window) LivedLong() bool {
	return w != nil && time.Since(w.started) > 8*time.Second
}

func startTracked(cmd *exec.Cmd) (*Window, error) {
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	w := &Window{cmd: cmd, started: time.Now(), done: make(chan struct{})}
	go func() {
		cmd.Wait()
		close(w.done)
	}()
	return w, nil
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

// appFlags open a window without address bar and keep timers running when
// the window is minimised (the page reports every few seconds that it is open).
func appFlags(url, profileDir string) []string {
	return []string{
		"--app=" + url,
		"--user-data-dir=" + profileDir,
		"--no-first-run",
		"--no-default-browser-check",
		"--disable-background-timer-throttling",
		"--disable-renderer-backgrounding",
		"--disable-backgrounding-occluded-windows",
		"--disable-features=Translate,msEdgeSidebarV2,msUndersideButton",
		"--window-size=1366,860",
		"--lang=pl",
	}
}
