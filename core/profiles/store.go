package profiles

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/faceair/clash-speedtest/core/appdata"
)

const (
	StoreFileName = "airports.json"
	CacheDirName  = "airports-cache"
	LegacyEnvFile = "run.local.env"
)

type Airport struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	URL       string    `json:"url"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
}

type Store struct {
	Airports []*Airport `json:"airports"`
}

type Paths struct {
	Dir string
}

func DefaultPaths() Paths {
	resolved, err := appdata.Resolve("")
	if err != nil {
		return Paths{}
	}
	return Paths{Dir: resolved.ProfileDir}
}

func (p Paths) StoreFile() string {
	return filepath.Join(p.Dir, StoreFileName)
}

func (p Paths) CacheDir() string {
	return filepath.Join(p.Dir, CacheDirName)
}

func (p Paths) CacheFile(id string) string {
	return filepath.Join(p.CacheDir(), id+".yaml")
}

func (p Paths) LegacyEnvFile() string {
	return filepath.Join(p.Dir, LegacyEnvFile)
}

func LoadStore(path string) (*Store, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Store{}, nil
		}
		return nil, err
	}
	var store Store
	if err := json.Unmarshal(data, &store); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if store.Airports == nil {
		store.Airports = []*Airport{}
	}
	if err := validateStore(&store); err != nil {
		return nil, fmt.Errorf("validate %s: %w", path, err)
	}
	return &store, nil
}

func SaveStore(path string, store *Store) error {
	if store == nil {
		store = &Store{}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o600)
}

func (s *Store) Add(airport *Airport) {
	s.Airports = append(s.Airports, airport)
}

func (s *Store) Remove(id string) *Airport {
	for i, airport := range s.Airports {
		if airport.ID == id {
			s.Airports = append(s.Airports[:i], s.Airports[i+1:]...)
			return airport
		}
	}
	return nil
}

func applyAirportEdit(airport *Airport, name, url string) (urlChanged bool) {
	if airport == nil {
		return false
	}
	if name != "" {
		airport.Name = name
	}
	urlChanged = airport.URL != url
	airport.URL = url
	if urlChanged {
		airport.UpdatedAt = time.Time{}
	}
	return urlChanged
}

func (s *Store) Get(id string) *Airport {
	for _, airport := range s.Airports {
		if airport.ID == id {
			return airport
		}
	}
	return nil
}

func (p Paths) HasCache(id string) bool {
	info, err := os.Stat(p.CacheFile(id))
	return err == nil && info.Size() > 0
}

func (p Paths) WriteCache(id string, body []byte) error {
	if err := os.MkdirAll(p.CacheDir(), 0o700); err != nil {
		return err
	}
	return os.WriteFile(p.CacheFile(id), body, 0o600)
}

func (p Paths) RemoveCache(id string) {
	_ = os.Remove(p.CacheFile(id))
}

func parseLegacyEnv(data []byte) string {
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(strings.TrimSuffix(raw, "\r"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		if strings.TrimSpace(key) != "CLASH_SPEEDTEST_CONFIG" {
			continue
		}
		val = strings.TrimSpace(val)
		if len(val) >= 2 {
			if (val[0] == '"' && val[len(val)-1] == '"') || (val[0] == '\'' && val[len(val)-1] == '\'') {
				val = val[1 : len(val)-1]
			}
		}
		return strings.TrimSpace(val)
	}
	return ""
}

func (p Paths) ImportLegacyIfEmpty(store *Store) bool {
	if store == nil || len(store.Airports) > 0 {
		return false
	}
	data, err := os.ReadFile(p.LegacyEnvFile())
	if err != nil {
		return false
	}
	url := parseLegacyEnv(data)
	if url == "" {
		return false
	}
	store.Add(&Airport{
		ID:   newAirportID(),
		Name: "导入的机场",
		URL:  url,
	})
	return true
}
