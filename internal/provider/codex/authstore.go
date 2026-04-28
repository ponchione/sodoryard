package codex

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

const sirtophamAuthStoreVersion = 1

var errCodexStoreStateNotFound = errors.New("codex auth state not found in Yard compatibility auth store")

type sirtophamAuthStore struct {
	Version        int                      `json:"version"`
	ActiveProvider string                   `json:"active_provider,omitempty"`
	Providers      map[string]codexAuthFile `json:"providers,omitempty"`
}

func sirtophamAuthStorePath(home string) string {
	return filepath.Join(home, ".sirtopham", "auth.json")
}

func codexCLIAuthPath(home string) string {
	if codexHome := strings.TrimSpace(os.Getenv("CODEX_HOME")); codexHome != "" {
		return filepath.Join(codexHome, "auth.json")
	}
	return filepath.Join(home, ".codex", "auth.json")
}

func readJSONFileLocked(path string, dst any) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_SH); err != nil {
		return fmt.Errorf("lock %s: %w", path, err)
	}
	defer syscall.Flock(int(file.Fd()), syscall.LOCK_UN)

	if err := json.NewDecoder(file).Decode(dst); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	return nil
}

func writeJSONFileAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create auth store directory: %w", err)
	}

	lockPath := path + ".lock"
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("open auth store lock file: %w", err)
	}
	defer lockFile.Close()

	locked := false
	for i := 0; i < 50; i++ {
		if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err == nil {
			locked = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !locked {
		return fmt.Errorf("timed out acquiring auth store lock for %s", path)
	}
	defer syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN)

	tmpFile, err := os.CreateTemp(filepath.Dir(path), ".auth-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary auth store file: %w", err)
	}
	tmpPath := tmpFile.Name()
	renamed := false
	defer func() {
		_ = tmpFile.Close()
		if !renamed {
			_ = os.Remove(tmpPath)
		}
	}()

	if err := os.Chmod(tmpPath, 0o600); err != nil {
		return fmt.Errorf("chmod temporary auth store file: %w", err)
	}
	if _, err := tmpFile.Write(data); err != nil {
		return fmt.Errorf("write temporary auth store file: %w", err)
	}
	if err := tmpFile.Sync(); err != nil {
		return fmt.Errorf("sync temporary auth store file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("close temporary auth store file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("replace auth store file: %w", err)
	}
	renamed = true
	return nil
}

func readCodexStore(storePath string) (*sirtophamAuthStore, error) {
	var store sirtophamAuthStore
	if err := readJSONFileLocked(storePath, &store); err != nil {
		return nil, err
	}
	if store.Version == 0 {
		store.Version = sirtophamAuthStoreVersion
	}
	if store.Providers == nil {
		store.Providers = make(map[string]codexAuthFile)
	}
	return &store, nil
}

func writeCodexStore(storePath string, auth codexAuthFile) error {
	store, err := readCodexStore(storePath)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		store = &sirtophamAuthStore{}
	}
	if store.Version == 0 {
		store.Version = sirtophamAuthStoreVersion
	}
	store.ActiveProvider = "codex"
	if store.Providers == nil {
		store.Providers = make(map[string]codexAuthFile)
	}
	store.Providers["codex"] = auth

	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal auth store: %w", err)
	}
	data = append(data, '\n')
	return writeJSONFileAtomic(storePath, data)
}

func readCodexStoreState(storePath string) (*codexAuthState, error) {
	store, err := readCodexStore(storePath)
	if err != nil {
		return nil, err
	}
	auth, ok := store.Providers["codex"]
	if !ok {
		return nil, errCodexStoreStateNotFound
	}
	return buildCodexAuthState(storePath, storePath, store.Version, store.ActiveProvider, false, auth)
}
