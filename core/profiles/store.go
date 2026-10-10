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

type Subscription struct {
	LastFailureRetryAt time.Time          `json:"last_failure_retry_at,omitempty"`
	LastFailureAt      time.Time          `json:"last_failure_at,omitempty"`
	LastFailureCode    string             `json:"last_failure_code,omitempty"`
	LastFailureMessage string             `json:"last_failure_message,omitempty"`
	Usage              *SubscriptionUsage `json:"usage,omitempty"`
	ID                 string             `json:"id"`
	Name               string             `json:"name"`
	URL                string             `json:"url"`
	Note               string             `json:"note,omitempty"`
	UpdatedAt          time.Time          `json:"updated_at,omitempty"`
}

type Airport struct {
	Maintenance   AirportMaintenance `json:"maintenance,omitempty"`
	ID            string             `json:"id"`
	Name          string             `json:"name"`
	URL           string             `json:"url,omitempty"`
	WebsiteURL    string             `json:"website_url,omitempty"`
	BackupURL     string             `json:"backup_url,omitempty"`
	Note          string             `json:"note,omitempty"`
	Subscriptions []*Subscription    `json:"subscriptions,omitempty"`
	UpdatedAt     time.Time          `json:"updated_at,omitempty"`
}

func (a *Airport) GetSubscription(id string) *Subscription {
	if a == nil {
		return nil
	}
	for _, sub := range a.Subscriptions {
		if sub.ID == id {
			return sub
		}
	}
	return nil
}

func (a *Airport) AddSubscription(sub *Subscription) {
	if a == nil || sub == nil {
		return
	}
	if sub.ID == "" {
		sub.ID = newAirportID()
	}
	a.Subscriptions = append(a.Subscriptions, sub)
	if a.URL == "" {
		a.URL = sub.URL
	}
}

func (a *Airport) RemoveSubscription(id string) *Subscription {
	if a == nil {
		return nil
	}
	for i, sub := range a.Subscriptions {
		if sub.ID == id {
			a.Subscriptions = append(a.Subscriptions[:i], a.Subscriptions[i+1:]...)
			if a.URL == sub.URL {
				if len(a.Subscriptions) > 0 {
					a.URL = a.Subscriptions[0].URL
				} else {
					a.URL = ""
				}
			}
			return sub
		}
	}
	return nil
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
	for _, ap := range store.Airports {
		if ap == nil {
			continue
		}
		if len(ap.Subscriptions) == 0 && ap.URL != "" {
			ap.Subscriptions = []*Subscription{
				{
					ID:        ap.ID,
					Name:      "默认订阅",
					URL:       ap.URL,
					UpdatedAt: ap.UpdatedAt,
				},
			}
		}
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
	for _, ap := range store.Airports {
		if ap == nil {
			continue
		}
		if len(ap.Subscriptions) > 0 && ap.URL == "" {
			ap.URL = ap.Subscriptions[0].URL
			if ap.UpdatedAt.IsZero() {
				ap.UpdatedAt = ap.Subscriptions[0].UpdatedAt
			}
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	file, err := os.CreateTemp(filepath.Dir(path), ".airports-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err = file.Write(data); err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(file.Name(), path)
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
	if len(airport.Subscriptions) > 0 {
		if airport.Subscriptions[0].URL != url {
			urlChanged = true
			airport.Subscriptions[0].URL = url
			airport.Subscriptions[0].UpdatedAt = time.Time{}
		}
		if name != "" && airport.Subscriptions[0].Name == "默认订阅" {
			// keep alias or unchanged
		}
	} else if url != "" {
		airport.Subscriptions = []*Subscription{
			{
				ID:   airport.ID,
				Name: "默认订阅",
				URL:  url,
			},
		}
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

func (s *Store) FindSubscription(id string) (*Airport, *Subscription) {
	for _, airport := range s.Airports {
		if airport == nil {
			continue
		}
		for _, sub := range airport.Subscriptions {
			if sub != nil && sub.ID == id {
				return airport, sub
			}
		}
		if airport.ID == id && len(airport.Subscriptions) > 0 {
			return airport, airport.Subscriptions[0]
		}
	}
	return nil, nil
}

func (p Paths) HasCache(id string) bool {
	info, err := os.Stat(p.CacheFile(id))
	return err == nil && info.Size() > 0
}

func (p Paths) WriteCache(id string, body []byte) error {
	if err := os.MkdirAll(p.CacheDir(), 0o700); err != nil {
		return err
	}
	return writeAtomicCache(p.CacheFile(id), body)
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

func writeAtomicCache(path string, body []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".cache-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(0o600); err != nil {
		_ = f.Close()
		return err
	}
	if _, err = f.Write(body); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(f.Name(), path)
}
