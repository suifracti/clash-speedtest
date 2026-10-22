package entry

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	// A node server rarely changes address, so a resolved host is reused for a
	// day unless the user asks for a refresh.
	HostTTL = 24 * time.Hour
	// An address' country and ISP are stable for much longer.
	AddressTTL = 30 * 24 * time.Hour
	// The local public position is the only value the instruction expires hourly.
	LocalTTL = time.Hour
)

type cacheFile struct {
	Version   int                `json:"version"`
	Hosts     map[string]Host    `json:"hosts,omitempty"`
	Addresses map[string]Address `json:"addresses,omitempty"`
	Local     *Local             `json:"local,omitempty"`
}

// Cache is a small JSON cache in the data directory: one entry per node host,
// one per entry address, one for this machine. It is safe for concurrent use.
type Cache struct {
	mu     sync.Mutex
	path   string
	loaded bool
	data   cacheFile
	dirty  bool
}

// Open returns a cache backed by path. A missing or unreadable file starts empty;
// cache failures never block a measurement.
func Open(path string) *Cache {
	return &Cache{path: path}
}

func (c *Cache) load() {
	if c.loaded {
		return
	}
	c.loaded = true
	c.data = cacheFile{Version: 1, Hosts: map[string]Host{}, Addresses: map[string]Address{}}
	if c.path == "" {
		return
	}
	raw, err := os.ReadFile(c.path)
	if err != nil {
		return
	}
	var stored cacheFile
	if json.Unmarshal(raw, &stored) != nil {
		return
	}
	stored.Version = 1
	if stored.Hosts == nil {
		stored.Hosts = map[string]Host{}
	}
	if stored.Addresses == nil {
		stored.Addresses = map[string]Address{}
	}
	c.data = stored
}

// Host returns a cached resolution younger than maxAge.
func (c *Cache) Host(host string, maxAge time.Duration) (Host, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.load()
	record, ok := c.data.Hosts[host]
	if !ok || record.Error != "" || len(record.Addresses) == 0 {
		return Host{}, false
	}
	if maxAge > 0 && time.Since(record.ResolvedAt) > maxAge {
		return Host{}, false
	}
	record.Source = "cache"
	return record, true
}

// PutHost stores a resolution and takes over the addresses it carries.
func (c *Cache) PutHost(record Host) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.load()
	if record.Host == "" {
		return
	}
	c.data.Hosts[record.Host] = record
	c.dirty = true
}

// Address returns a cached picture younger than maxAge.
func (c *Cache) Address(ip string, maxAge time.Duration) (Address, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.load()
	record, ok := c.data.Addresses[ip]
	if !ok || record.CountryCode == "" {
		return Address{}, false
	}
	if maxAge > 0 && time.Since(record.QueriedAt) > maxAge {
		return Address{}, false
	}
	return record, true
}

// PutAddress stores one address picture.
func (c *Cache) PutAddress(record Address) {
	if record.IP == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.load()
	c.data.Addresses[record.IP] = record
	c.dirty = true
}

// Local returns this machine's cached position younger than maxAge.
func (c *Cache) Local(maxAge time.Duration) (Local, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.load()
	if c.data.Local == nil || c.data.Local.CountryCode == "" {
		return Local{}, false
	}
	if maxAge > 0 && time.Since(c.data.Local.QueriedAt) > maxAge {
		return Local{}, false
	}
	return *c.data.Local, true
}

// PutLocal stores this machine's position.
func (c *Cache) PutLocal(record Local) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.load()
	c.data.Local = &record
	c.dirty = true
}

// Save writes the cache atomically when something changed.
func (c *Cache) Save() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.load()
	if !c.dirty || c.path == "" {
		return nil
	}
	c.data.Version = 1
	raw, err := json.MarshalIndent(c.data, "", " ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(c.path), 0o755); err != nil {
		return err
	}
	temp := c.path + ".tmp"
	if err := os.WriteFile(temp, raw, 0o600); err != nil {
		return err
	}
	if err := os.Rename(temp, c.path); err != nil {
		return err
	}
	c.dirty = false
	return nil
}
