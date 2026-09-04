package message

import (
	"bufio"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Message struct {
	ID        uint64 `json:"id"`
	Epoch     uint64 `json:"epoch"`
	Sender    string `json:"sender,omitempty"`
	Data      string `json:"data"`
	Timestamp int64  `json:"timestamp"`
}

type Store struct {
	mu       sync.RWMutex
	path     string
	messages []Message
	nextID   uint64
	now      func() time.Time
}

func OpenStore(path string) (*Store, error) {
	store := &Store{path: path, nextID: 1, now: time.Now}
	if err := store.load(); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *Store) Add(epoch uint64, sender, data string) (Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	message := Message{
		ID:        s.nextID,
		Epoch:     epoch,
		Sender:    sender,
		Data:      data,
		Timestamp: s.now().UnixMilli(),
	}
	if err := appendMessage(s.path, message); err != nil {
		return Message{}, err
	}
	s.messages = append(s.messages, message)
	s.nextID++
	return message, nil
}

func (s *Store) Between(from, to int64) []Message {
	s.mu.RLock()
	defer s.mu.RUnlock()

	messages := make([]Message, 0)
	for _, message := range s.messages {
		if message.Timestamp >= from && message.Timestamp <= to {
			messages = append(messages, message)
		}
	}
	return messages
}

func (s *Store) load() error {
	f, err := os.Open(s.path)
	if errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
			return err
		}
		f, err = os.OpenFile(s.path, os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}
		return f.Close()
	}
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	for line := 1; scanner.Scan(); line++ {
		parts := strings.SplitN(scanner.Text(), "\t", 6)
		if len(parts) != 6 || parts[0] != "message" {
			return fmt.Errorf("invalid message state at line %d", line)
		}
		id, err := strconv.ParseUint(parts[1], 10, 64)
		if err != nil {
			return fmt.Errorf("parse message ID at line %d: %w", line, err)
		}
		timestamp, err := strconv.ParseInt(parts[2], 10, 64)
		if err != nil {
			return fmt.Errorf("parse timestamp at line %d: %w", line, err)
		}
		epoch, err := strconv.ParseUint(parts[3], 10, 64)
		if err != nil {
			return fmt.Errorf("parse message epoch at line %d: %w", line, err)
		}
		senderBytes, err := base64.StdEncoding.DecodeString(parts[4])
		if err != nil {
			return fmt.Errorf("parse sender at line %d: %w", line, err)
		}
		if id != s.nextID {
			return fmt.Errorf("non-contiguous message ID at line %d", line)
		}
		s.messages = append(s.messages, Message{
			ID: id, Epoch: epoch, Sender: string(senderBytes), Data: parts[5], Timestamp: timestamp,
		})
		s.nextID++
	}
	return scanner.Err()
}

func appendMessage(path string, message Message) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	sender := base64.StdEncoding.EncodeToString([]byte(message.Sender))
	if _, err := fmt.Fprintf(f, "message\t%d\t%d\t%d\t%s\t%s\n", message.ID, message.Timestamp, message.Epoch, sender, message.Data); err != nil {
		return err
	}
	return f.Sync()
}
