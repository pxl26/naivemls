package e2ee

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

var ErrEpochMismatch = errors.New("epoch mismatch")

type Commit struct {
	Epoch uint64 `json:"epoch"`
	Data  string `json:"data"`
}

type Store struct {
	mu           sync.RWMutex
	path         string
	currentEpoch uint64
	commits      []Commit
}

func OpenStore(path string) (*Store, error) {
	store := &Store{path: path}
	if err := store.load(); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *Store) CurrentEpoch() uint64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.currentEpoch
}

func (s *Store) CommitsAfter(localEpoch uint64) (uint64, []Commit, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if localEpoch > s.currentEpoch {
		return s.currentEpoch, nil, ErrEpochMismatch
	}

	commits := make([]Commit, 0, s.currentEpoch-localEpoch)
	for _, commit := range s.commits {
		if commit.Epoch > localEpoch {
			commits = append(commits, commit)
		}
	}
	return s.currentEpoch, commits, nil
}

func (s *Store) AddProposal(commit Commit) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	expected := s.currentEpoch + 1
	if commit.Epoch != expected {
		return ErrEpochMismatch
	}

	commits := append(append([]Commit(nil), s.commits...), commit)
	if err := writeState(s.path, commit.Epoch, commits); err != nil {
		return err
	}
	s.currentEpoch = commit.Epoch
	s.commits = commits
	return nil
}

func (s *Store) load() error {
	f, err := os.Open(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return writeState(s.path, 0, nil)
	}
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return err
		}
		return fmt.Errorf("empty E2EE state file")
	}
	header := strings.Split(scanner.Text(), "\t")
	if len(header) != 2 || header[0] != "epoch" {
		return fmt.Errorf("invalid E2EE state header")
	}
	currentEpoch, err := strconv.ParseUint(header[1], 10, 64)
	if err != nil {
		return fmt.Errorf("parse current epoch: %w", err)
	}

	var commits []Commit
	for line := 2; scanner.Scan(); line++ {
		parts := strings.SplitN(scanner.Text(), "\t", 3)
		if len(parts) != 3 || parts[0] != "commit" {
			return fmt.Errorf("invalid E2EE state at line %d", line)
		}
		epoch, err := strconv.ParseUint(parts[1], 10, 64)
		if err != nil {
			return fmt.Errorf("parse commit epoch at line %d: %w", line, err)
		}
		if epoch != uint64(len(commits)+1) {
			return fmt.Errorf("non-contiguous commit epoch at line %d", line)
		}
		commits = append(commits, Commit{Epoch: epoch, Data: parts[2]})
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	if uint64(len(commits)) != currentEpoch {
		return fmt.Errorf("current epoch %d does not match %d stored commits", currentEpoch, len(commits))
	}

	s.currentEpoch = currentEpoch
	s.commits = commits
	return nil
}

func writeState(path string, currentEpoch uint64, commits []Commit) (err error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(dir, ".e2ee-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		tmp.Close()
		if err != nil {
			_ = os.Remove(tmpName)
		}
	}()

	w := bufio.NewWriter(tmp)
	if _, err = fmt.Fprintf(w, "epoch\t%d\n", currentEpoch); err != nil {
		return err
	}
	for _, commit := range commits {
		if _, err = fmt.Fprintf(w, "commit\t%d\t%s\n", commit.Epoch, commit.Data); err != nil {
			return err
		}
	}
	if err = w.Flush(); err != nil {
		return err
	}
	if err = tmp.Sync(); err != nil {
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	if err = os.Chmod(tmpName, 0o644); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
