// Wypożyczalnia – vehicle rental management. A single program: it keeps the
// data in a local SQLite database and shows its interface in an Edge window.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"vehicle-rent-app/internal/db"
	"vehicle-rent-app/internal/launcher"
	"vehicle-rent-app/internal/store"
	"vehicle-rent-app/internal/web"
)

var version = "dev"

const defaultPort = 47800

func main() {
	if err := run(); err != nil {
		log.Printf("błąd: %v", err)
		launcher.ShowError("Wypożyczalnia – błąd", err.Error())
		os.Exit(1)
	}
}

func run() error {
	dataDir := flag.String("data", "", "folder z danymi (domyślnie Dokumenty\\Wypożyczalnia)")
	port := flag.Int("port", defaultPort, "port lokalnego serwera")
	noWindow := flag.Bool("no-window", false, "nie otwieraj okna (tylko serwer)")
	flag.Parse()

	if *dataDir == "" {
		d, err := launcher.DataDir()
		if err != nil {
			return fmt.Errorf("nie można ustalić folderu danych: %w", err)
		}
		*dataDir = d
	}
	if err := os.MkdirAll(*dataDir, 0o755); err != nil {
		return fmt.Errorf("nie można utworzyć folderu %s: %w", *dataDir, err)
	}
	setupLog(filepath.Join(*dataDir, "dziennik.log"))
	log.Printf("start wersja %s, dane %s", version, *dataDir)

	profileDir := filepath.Join(*dataDir, ".okno")

	// Already running? Then just open another window to it.
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", *port))
	if err != nil {
		if running(*port) {
			if !*noWindow {
				_, err := launcher.OpenWindow(fmt.Sprintf("http://127.0.0.1:%d/", *port), profileDir)
				return err
			}
			return errors.New("program już działa")
		}
		// port taken by something else: use any free port
		if ln, err = net.Listen("tcp", "127.0.0.1:0"); err != nil {
			return err
		}
	}

	dbPath := filepath.Join(*dataDir, "wypozyczalnia.db")
	backupDir := filepath.Join(*dataDir, "kopie")
	// Back up the existing database before opening (and migrating) it.
	if _, err := os.Stat(dbPath); err == nil {
		if d, err := db.Open(dbPath); err == nil {
			if _, err := db.Backup(d, backupDir, 30); err != nil {
				log.Printf("kopia zapasowa: %v", err)
			}
			d.Close()
		} else {
			return err
		}
	}
	d, err := db.Open(dbPath)
	if err != nil {
		return err
	}
	defer d.Close()

	srv, err := web.New(store.New(d), web.Options{
		DataDir:    *dataDir,
		BackupDir:  backupDir,
		Backup:     func() (string, error) { return db.Backup(d, backupDir, 30) },
		OpenFolder: launcher.OpenFolder,
		Version:    version,
	})
	if err != nil {
		return err
	}
	httpSrv := &http.Server{Handler: srv, ReadHeaderTimeout: 10 * time.Second}
	serveErr := make(chan error, 1)
	go func() { serveErr <- httpSrv.Serve(ln) }()

	url := fmt.Sprintf("http://127.0.0.1:%d/", ln.Addr().(*net.TCPAddr).Port)
	log.Printf("serwer %s", url)
	if *noWindow {
		fmt.Println("Wypożyczalnia działa:", url)
		return <-serveErr
	}

	win, err := launcher.OpenWindow(url, profileDir)
	if err != nil {
		return fmt.Errorf("nie można otworzyć okna programu: %w", err)
	}

	// Run until the window is closed: either our browser process ends, or no
	// window has reported in for a while.
	started := time.Now()
	tick := time.NewTicker(5 * time.Second)
	defer tick.Stop()
	done := win.Done()
	for {
		select {
		case err := <-serveErr:
			return err
		case <-done:
			done = nil
			if win.LivedLong() {
				log.Printf("okno zamknięte")
				return shutdown(httpSrv)
			}
			// the browser handed the window over to an already running copy;
			// fall back to heartbeats
		case <-tick.C:
			last := srv.LastHeartbeat()
			if last.IsZero() {
				last = started
			}
			limit := 90 * time.Second
			if srv.LastHeartbeat().IsZero() {
				limit = 3 * time.Minute // first start of the browser can be slow
			}
			if time.Since(last) > limit {
				log.Printf("brak okien od %s – zamykanie", time.Since(last).Round(time.Second))
				return shutdown(httpSrv)
			}
		}
	}
}

func shutdown(s *http.Server) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	s.Shutdown(ctx)
	return nil
}

// running checks whether our program answers on the port.
func running(port int) bool {
	c := http.Client{Timeout: 2 * time.Second}
	resp, err := c.Get(fmt.Sprintf("http://127.0.0.1:%d/api/ping", port))
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 100))
	return strings.TrimSpace(string(b)) == web.AppID
}

func setupLog(path string) {
	if st, err := os.Stat(path); err == nil && st.Size() > 5<<20 {
		os.Rename(path, path+".1")
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	log.SetOutput(io.MultiWriter(f, os.Stderr))
}
