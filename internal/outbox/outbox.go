package outbox

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func dir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".agent-memory", "outbox"), nil
}

func Write(data any) error {
	d, err := dir()
	if err != nil {
		return fmt.Errorf("get outbox dir: %w", err)
	}
	if err := os.MkdirAll(d, 0700); err != nil {
		return fmt.Errorf("create outbox dir: %w", err)
	}

	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal outbox entry: %w", err)
	}

	var m map[string]json.RawMessage
	orgStr, projectStr := "unknown", "unknown"
	if json.Unmarshal(b, &m) == nil {
		if raw, ok := m["org"]; ok {
			_ = json.Unmarshal(raw, &orgStr)
		}
		if raw, ok := m["project"]; ok {
			_ = json.Unmarshal(raw, &projectStr)
		}
	}

	ts := time.Now().UTC().Format("20060102-150405.000")
	filename := fmt.Sprintf("%s.%s.%s.json", ts, orgStr, projectStr)
	return os.WriteFile(filepath.Join(d, filename), b, 0600)
}

func Count() (int, error) {
	d, err := dir()
	if err != nil {
		return 0, err
	}
	entries, err := os.ReadDir(d)
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read outbox dir: %w", err)
	}
	n := 0
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
			n++
		}
	}
	return n, nil
}

type Entry struct {
	Filename string
	Path     string
	Data     json.RawMessage
}

func ReadAll() ([]Entry, error) {
	d, err := dir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(d)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read outbox dir: %w", err)
	}
	var result []Entry
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		p := filepath.Join(d, e.Name())
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		result = append(result, Entry{Filename: e.Name(), Path: p, Data: data})
	}
	return result, nil
}

func Delete(path string) error {
	return os.Remove(path)
}
