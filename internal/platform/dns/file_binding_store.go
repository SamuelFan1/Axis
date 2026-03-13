package dns

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type FileBindingStore struct {
	stateDir         string
	nodesDir         string
	nextSequencePath string

	mu sync.Mutex
}

func NewFileBindingStore(stateDir string) BindingStore {
	trimmedStateDir := strings.TrimSpace(stateDir)
	return &FileBindingStore{
		stateDir:         trimmedStateDir,
		nodesDir:         filepath.Join(trimmedStateDir, "nodes"),
		nextSequencePath: filepath.Join(trimmedStateDir, "next-sequence"),
	}
}

func (s *FileBindingStore) Load(nodeUUID string) (*Binding, error) {
	nodeUUID = strings.TrimSpace(nodeUUID)
	if nodeUUID == "" {
		return nil, fmt.Errorf("node uuid is required")
	}

	binding, err := s.loadBindingFile(s.nodePath(nodeUUID))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	return &binding, nil
}

func (s *FileBindingStore) Save(binding Binding) error {
	normalized, err := normalizeBinding(binding)
	if err != nil {
		return err
	}

	payload, err := json.MarshalIndent(normalized, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal dns binding: %w", err)
	}
	payload = append(payload, '\n')

	return writeFileAtomic(s.nodePath(normalized.NodeUUID), payload)
}

func (s *FileBindingStore) List() ([]Binding, error) {
	entries, err := os.ReadDir(s.nodesDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("list dns binding files: %w", err)
	}

	items := make([]Binding, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		binding, err := s.loadBindingFile(filepath.Join(s.nodesDir, entry.Name()))
		if err != nil {
			return nil, err
		}
		items = append(items, binding)
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].NodeUUID < items[j].NodeUUID
	})
	return items, nil
}

func (s *FileBindingStore) ReserveNextSequence(prefix string) (int, error) {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return 0, fmt.Errorf("dns prefix is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	nextFromFile, err := s.readNextSequence()
	if err != nil {
		return 0, err
	}

	maxSequence, err := s.maxSequenceFromBindings(prefix)
	if err != nil {
		return 0, err
	}

	nextSequence := nextFromFile
	if nextSequence <= maxSequence {
		nextSequence = maxSequence + 1
	}
	if nextSequence <= 0 {
		nextSequence = 1
	}

	if err := s.writeNextSequence(nextSequence + 1); err != nil {
		return 0, err
	}
	return nextSequence, nil
}

func (s *FileBindingStore) nodePath(nodeUUID string) string {
	return filepath.Join(s.nodesDir, strings.TrimSpace(nodeUUID)+".json")
}

func (s *FileBindingStore) loadBindingFile(path string) (Binding, error) {
	payload, err := os.ReadFile(path)
	if err != nil {
		return Binding{}, err
	}

	var binding Binding
	if err := json.Unmarshal(payload, &binding); err != nil {
		return Binding{}, fmt.Errorf("decode dns binding %s: %w", path, err)
	}

	normalized, err := normalizeBinding(binding)
	if err != nil {
		return Binding{}, fmt.Errorf("invalid dns binding %s: %w", path, err)
	}

	expectedNodeUUID := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	if expectedNodeUUID != "" && normalized.NodeUUID != expectedNodeUUID {
		return Binding{}, fmt.Errorf("dns binding file %s does not match node uuid %s", path, expectedNodeUUID)
	}

	return normalized, nil
}

func (s *FileBindingStore) readNextSequence() (int, error) {
	payload, err := os.ReadFile(s.nextSequencePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, nil
		}
		return 0, fmt.Errorf("read next dns sequence: %w", err)
	}

	value := strings.TrimSpace(string(payload))
	if value == "" {
		return 0, nil
	}

	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("parse next dns sequence: %w", err)
	}
	if parsed < 0 {
		return 0, fmt.Errorf("next dns sequence must be non-negative")
	}
	return parsed, nil
}

func (s *FileBindingStore) writeNextSequence(value int) error {
	payload := []byte(strconv.Itoa(value) + "\n")
	if err := writeFileAtomic(s.nextSequencePath, payload); err != nil {
		return fmt.Errorf("write next dns sequence: %w", err)
	}
	return nil
}

func (s *FileBindingStore) maxSequenceFromBindings(prefix string) (int, error) {
	entries, err := os.ReadDir(s.nodesDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, nil
		}
		return 0, fmt.Errorf("list dns binding files: %w", err)
	}

	maxSequence := 0
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		binding, err := s.loadBindingFile(filepath.Join(s.nodesDir, entry.Name()))
		if err != nil {
			return 0, err
		}

		sequence, ok := ParseDNSSequence(prefix, binding.DNSLabel)
		if ok && sequence > maxSequence {
			maxSequence = sequence
		}
	}

	return maxSequence, nil
}

func normalizeBinding(binding Binding) (Binding, error) {
	binding.NodeUUID = strings.TrimSpace(binding.NodeUUID)
	binding.DNSLabel = strings.TrimSpace(binding.DNSLabel)
	binding.DNSName = strings.TrimSpace(binding.DNSName)
	binding.LastPublicIP = strings.TrimSpace(binding.LastPublicIP)
	if binding.NodeUUID == "" {
		return Binding{}, fmt.Errorf("node uuid is required")
	}
	if binding.DNSLabel == "" {
		return Binding{}, fmt.Errorf("dns label is required")
	}
	if binding.DNSName == "" {
		return Binding{}, fmt.Errorf("dns name is required")
	}
	if binding.UpdatedAt.IsZero() {
		binding.UpdatedAt = time.Now().UTC()
	} else {
		binding.UpdatedAt = binding.UpdatedAt.UTC()
	}
	return binding, nil
}

func writeFileAtomic(path string, payload []byte) (err error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create dns state directory: %w", err)
	}

	tempFile, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp dns state file: %w", err)
	}

	tempPath := tempFile.Name()
	defer func() {
		if err != nil {
			_ = os.Remove(tempPath)
		}
	}()

	if _, err = tempFile.Write(payload); err != nil {
		_ = tempFile.Close()
		return fmt.Errorf("write temp dns state file: %w", err)
	}
	if err = tempFile.Chmod(0o644); err != nil {
		_ = tempFile.Close()
		return fmt.Errorf("chmod temp dns state file: %w", err)
	}
	if err = tempFile.Close(); err != nil {
		return fmt.Errorf("close temp dns state file: %w", err)
	}
	if err = os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("rename temp dns state file: %w", err)
	}
	return nil
}
