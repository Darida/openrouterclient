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

// Open validates any existing file up front so a corrupt history fails at
// construction, not mid-generation.
func Open(path string) (*Store, error) {
	s := &Store{path: path}
	if err := s.withLock(func() error {
		_, err := s.load()
		return err
	}); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) Append(entry Entry) error {
	return s.withLock(func() error {
		entries, err := s.load()
		if err != nil {
			return err
		}
		return s.save(append(entries, entry))
	})
}

// Exclusions weighs each failure by whether it was recorded under tag.
func (s *Store) Exclusions(now time.Time, tag string) (Exclusions, error) {
	var entries []Entry
	if err := s.withLock(func() error {
		var err error
		entries, err = s.load()
		return err
	}); err != nil {
		return Exclusions{}, err
	}
	return computeExclusions(entries, now, tag)
}

// RecordManual rates a generator's or a reviewer's generation. Rating a
// reviewer low also clears the automatic rating its verdict gave. It returns
// an error for an invalid quality, an empty id, an id with no automatic
// entry, an id already rated, or a history file it can't read or write.
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
	return s.withLock(func() error {
		entries, err := s.load()
		if err != nil {
			return err
		}
		var rated *EntryFields
		for _, e := range entries {
			f := e.Fields()
			if f.GenerationID != generationID {
				continue
			}
			if f.Source == SourceManual {
				return fmt.Errorf("history: generation %q is already rated", generationID)
			}
			rated = &f
		}
		if rated == nil {
			return fmt.Errorf("history: no generation %q in history", generationID)
		}
		entry, err := NewEntry(EntryFields{
			Timestamp:      now.UTC(),
			Model:          rated.Model,
			GenerationID:   generationID,
			Role:           rated.Role,
			Source:         SourceManual,
			Outcome:        OutcomeSuccess,
			Quality:        q,
			TargetQuality:  rated.TargetQuality,
			TimeoutSeconds: rated.TimeoutSeconds,
			Reason:         reason,
			Tag:            rated.Tag,
		})
		if err != nil {
			return err
		}
		if q == model.QualityLow && rated.ReviewedGenerationID != "" {
			if err := unrateReviewed(entries, rated.ReviewedGenerationID, generationID, reason); err != nil {
				return err
			}
		}
		return s.save(append(entries, entry))
	})
}

// unrateReviewed clears the automatic rating a rejected reviewer gave, in
// place, so it stops counting toward exclusion. Manual ratings stay.
func unrateReviewed(entries []Entry, reviewedID, reviewerID, reason string) error {
	for i, e := range entries {
		f := e.Fields()
		if f.GenerationID != reviewedID || f.Role != RoleGenerator || f.Source != SourceAuto {
			continue
		}
		f.Quality = ""
		f.Reason = fmt.Sprintf("unrated: reviewer %s rated low: %s", reviewerID, reason)
		unrated, err := NewEntry(f)
		if err != nil {
			return err
		}
		entries[i] = unrated
		return nil
	}
	return fmt.Errorf("history: reviewer %s links to generation %q, which has no automatic generator entry", reviewerID, reviewedID)
}

func (s *Store) withLock(fn func() error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	lock, err := os.OpenFile(s.path+".lock", os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return fmt.Errorf("history: open lock file: %w", err)
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("history: flock: %w", err)
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	return fn()
}

// A missing file is an empty history; the first Append creates it.
func (s *Store) load() ([]Entry, error) {
	raw, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("history: read %s: %w", s.path, err)
	}
	var entries []Entry
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil, fmt.Errorf("history: %s is not a valid history file: %w", s.path, err)
	}
	return entries, nil
}

func (s *Store) save(entries []Entry) error {
	raw, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return fmt.Errorf("history: marshal: %w", err)
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, append(raw, '\n'), 0o644); err != nil {
		return fmt.Errorf("history: write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return fmt.Errorf("history: rename %s: %w", tmp, err)
	}
	return nil
}
