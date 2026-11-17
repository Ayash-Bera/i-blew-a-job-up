package db

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"firstrole/internal/models"
)

var ErrReadOnly = errors.New("snapshot storage is read-only; configure Postgres for refresh")
var ErrLeaseLost = errors.New("refresh lease is no longer owned by this run")

//go:embed bundled-snapshot.json
var bundledSnapshot []byte

type Store interface {
	Load(context.Context, bool) (models.State, error)
	Acquire(context.Context, string, time.Duration) (bool, error)
	Renew(context.Context, string, time.Duration) error
	Release(context.Context, string) error
	Save(context.Context, models.State, string) error
	PutRun(context.Context, models.Run) error
	GetTag(context.Context, string) (*models.TagRecord, error)
	PutTag(context.Context, models.TagRecord) error
	PutUsage(context.Context, models.Usage) error
	Usage(context.Context) ([]models.Usage, error)
}

func Empty() models.State {
	return models.State{Observations: map[string]models.Observation{}, Jobs: []models.Job{}, Sources: []models.SourceView{}, Runs: []models.Run{}, Evaluation: models.Evaluation{SampleSize: 50}, Mode: "live"}
}

type Memory struct {
	mu       sync.Mutex
	State    models.State
	tags     map[string]models.TagRecord
	usage    []models.Usage
	owner    string
	until    time.Time
	readOnly bool
}

func NewMemory() *Memory {
	return &Memory{State: Empty(), tags: map[string]models.TagRecord{}, usage: []models.Usage{}}
}
func OpenSnapshot(path string) (*Memory, error) {
	b := bundledSnapshot
	var err error
	if path != "embedded" {
		b, err = os.ReadFile(path)
	}
	if err != nil {
		return nil, fmt.Errorf("read snapshot: %w", err)
	}
	m := NewMemory()
	if err = json.Unmarshal(b, &m.State); err != nil {
		return nil, fmt.Errorf("decode snapshot: %w", err)
	}
	m.State.Mode = "snapshot_read_only"
	m.readOnly = true
	return m, nil
}
func (m *Memory) Load(_ context.Context, full bool) (models.State, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, err := json.Marshal(m.State)
	if err != nil {
		return models.State{}, err
	}
	var s models.State
	err = json.Unmarshal(b, &s)
	if !full {
		s.Observations = nil
	}
	return s, err
}
func (m *Memory) Acquire(_ context.Context, owner string, ttl time.Duration) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.readOnly {
		return false, ErrReadOnly
	}
	if m.owner != "" && time.Now().Before(m.until) {
		return false, nil
	}
	m.owner = owner
	m.until = time.Now().Add(ttl)
	return true, nil
}
func (m *Memory) Renew(_ context.Context, owner string, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.owner != owner || time.Now().After(m.until) {
		return ErrLeaseLost
	}
	m.until = time.Now().Add(ttl)
	return nil
}
func (m *Memory) Release(_ context.Context, owner string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.owner == owner {
		m.owner = ""
	}
	return nil
}
func (m *Memory) Save(_ context.Context, s models.State, owner string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.readOnly {
		return ErrReadOnly
	}
	if m.owner != owner || time.Now().After(m.until) {
		return ErrLeaseLost
	}
	s.Runs = m.State.Runs
	m.State = s
	return nil
}
func (m *Memory) PutRun(_ context.Context, r models.Run) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.readOnly {
		return ErrReadOnly
	}
	for i := range m.State.Runs {
		if m.State.Runs[i].ID == r.ID {
			m.State.Runs[i] = r
			return nil
		}
	}
	m.State.Runs = append([]models.Run{r}, m.State.Runs...)
	return nil
}
func (m *Memory) GetTag(_ context.Context, key string) (*models.TagRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.tags[key]
	if !ok {
		return nil, nil
	}
	return &v, nil
}
func (m *Memory) PutTag(_ context.Context, t models.TagRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.readOnly {
		return ErrReadOnly
	}
	m.tags[t.Key] = t
	return nil
}
func (m *Memory) PutUsage(_ context.Context, u models.Usage) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.readOnly {
		return ErrReadOnly
	}
	for i := range m.usage {
		if m.usage[i].ID == u.ID {
			m.usage[i] = u
			return nil
		}
	}
	m.usage = append(m.usage, u)
	return nil
}
func (m *Memory) Usage(_ context.Context) ([]models.Usage, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]models.Usage{}, m.usage...), nil
}
