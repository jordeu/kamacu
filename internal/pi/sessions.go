// Package pi handles the Pi CLI interfaces used by Kamacu.
package pi

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

// SessionRoot isolates transcripts by database instance, like Kamacu artifacts.
func SessionRoot(db *sql.DB) (string, error) {
	var seq int
	var name, path string
	if err := db.QueryRow(`PRAGMA database_list`).Scan(&seq, &name, &path); err != nil {
		return "", err
	}
	if name != "main" || !filepath.IsAbs(path) {
		return "", errors.New("Pi sessions require a file-backed Kamacu database")
	}
	path, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	return path + ".pi-sessions", nil
}

func sessionDir(root string, taskID int64, id string) (string, error) {
	parsed, err := uuid.Parse(id)
	if root == "" || err != nil || parsed.String() != id {
		return "", errors.New("missing or invalid Pi session ID")
	}
	scope := "scratchpad"
	if taskID > 0 {
		scope = fmt.Sprintf("task-%d", taskID)
	}
	return filepath.Join(root, scope, id), nil
}

func CreateSessionDir(root string, taskID int64, id string) (string, error) {
	dir, err := sessionDir(root, taskID, id)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(dir), 0700); err != nil {
		return "", err
	}
	if err := os.Mkdir(dir, 0700); err != nil {
		return "", err
	}
	return dir, nil
}

// ResumeFile returns the exact transcript Pi wrote for this launch.
func ResumeFile(root string, taskID int64, id, cwd string) (string, error) {
	dir, err := sessionDir(root, taskID, id)
	if err != nil {
		return "", err
	}
	realDir, err := filepath.EvalSymlinks(dir)
	if err != nil || realDir != dir {
		return "", errors.New("Pi session directory is unavailable")
	}
	currentDir, err := filepath.EvalSymlinks(cwd)
	if err != nil {
		return "", err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	var match string
	for _, entry := range entries {
		if !entry.Type().IsRegular() || !strings.HasSuffix(entry.Name(), "_"+id+".jsonl") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		if match != "" || !validHeader(path, id, currentDir) {
			return "", errors.New("Pi session transcript is ambiguous or invalid")
		}
		match = path
	}
	if match == "" {
		return "", errors.New("Pi session transcript is missing")
	}
	return match, nil
}

func CanResume(root string, taskID int64, id, cwd string) bool {
	_, err := ResumeFile(root, taskID, id, cwd)
	return err == nil
}

func validHeader(path, id, cwd string) bool {
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer file.Close()
	var header struct {
		Type string `json:"type"`
		ID   string `json:"id"`
		CWD  string `json:"cwd"`
	}
	scanner := bufio.NewScanner(file)
	if !scanner.Scan() || json.Unmarshal(scanner.Bytes(), &header) != nil ||
		header.Type != "session" || header.ID != id || !filepath.IsAbs(header.CWD) {
		return false
	}
	recorded, err := filepath.EvalSymlinks(header.CWD)
	return err == nil && cwd == recorded
}
