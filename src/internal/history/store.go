package history

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"syscall"
	"time"

	"github.com/Darida/openrouterclient/src/internal/quality"
	"github.com/Darida/openrouterclient/src/model"
)

// Store serializes read-modify-write within the process with a mutex, and
// across processes with an flock on a sibling ".lock" file.
type Store struct {
	path string
	mu   sync.Mutex
}

// Open validates any existing file up front so a corrupt history panics at
// construction, not mid-generation.
func Open(path string) *Store {
	s := &Store{path: path}
	s.withLock(func() { s.load() })
	return s
}

func (s *Store) Append(entry Entry) {
	s.withLock(func() {
		entries := s.load()
		s.save(append(entries, entry))
	})
}

// Exclusions weighs each failure by whether it was recorded under tag.
func (s *Store) Exclusions(now time.Time, tag string) Exclusions {
	var entries []Entry
	s.withLock(func() { entries = s.load() })
	return computeExclusions(entries, now, tag)
}

// RecordManual returns an error for caller mistakes: an invalid quality, an
// empty id, an id with no automatic generator entry, or an id already rated.
func (s *Store) RecordManual(generationID string, q model.Quality, reason string, now time.Time) error {
	if !quality.IsRating(q) {
		return fmt.Errorf("history: manual quality must be high, medium, or low, got %q", q)
	}
	if reason == "" {
		return errors.New("history: manual rating needs a reason")
	}
	if generationID == "" {
		return errors.New("history: manual rating needs a generation id")
	}
	var err error
	s.withLock(func() {
		entries := s.load()
		var rated *EntryFields
		for _, e := range entries {
			f := e.Fields()
			if f.GenerationID != generationID || f.Role != RoleGenerator {
				continue
			}
			if f.Source == SourceManual {
				err = fmt.Errorf("history: generation %q is already rated", generationID)
				return
			}
			rated = &f
		}
		if rated == nil {
			err = fmt.Errorf("history: no generation %q in history", generationID)
			return
		}
		entry := NewEntry(EntryFields{
			Timestamp:      now.UTC(),
			Model:          rated.Model,
			GenerationID:   generationID,
			Role:           RoleGenerator,
			Source:         SourceManual,
			Outcome:        OutcomeSuccess,
			Quality:        q,
			TargetQuality:  rated.TargetQuality,
			TimeoutSeconds: rated.TimeoutSeconds,
			Reason:         reason,
			Tag:            rated.Tag,
		})
		s.save(append(entries, entry))
	})
	return err
}

func (s *Store) withLock(fn func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	lock, err := os.OpenFile(s.path+".lock", os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		panic(fmt.Sprintf("history: open lock file: %v", err))
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		panic(fmt.Sprintf("history: flock: %v", err))
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	fn()
}

// A missing file is an empty history; the first Append creates it.
func (s *Store) load() []Entry {
	raw, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		panic(fmt.Sprintf("history: read %s: %v", s.path, err))
	}
	var entries []Entry
	if err := json.Unmarshal(raw, &entries); err != nil {
		panic(fmt.Sprintf("history: %s is not a valid history file: %v", s.path, err))
	}
	return entries
}

func (s *Store) save(entries []Entry) {
	raw, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		panic(fmt.Sprintf("history: marshal: %v", err))
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, append(raw, '\n'), 0o644); err != nil {
		panic(fmt.Sprintf("history: write %s: %v", tmp, err))
	}
	if err := os.Rename(tmp, s.path); err != nil {
		panic(fmt.Sprintf("history: rename %s: %v", tmp, err))
	}
}
