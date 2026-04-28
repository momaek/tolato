// Package geoip resolves IPs to country/city/ASN using MaxMind GeoLite2 mmdb files.
//
// Data files are sourced from the P3TERX/GeoLite.mmdb mirror (which repackages
// MaxMind's official GeoLite2 data). They are downloaded on first start and
// refreshed periodically by Service.Refresh.
//
// MaxMind GeoLite2 data is licensed under CC-BY-SA-4.0 — attribution must be
// shown wherever the resulting region/ASN data is surfaced.
package geoip

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/oschwald/maxminddb-golang"
)

const (
	cityFileName = "GeoLite2-City.mmdb"
	asnFileName  = "GeoLite2-ASN.mmdb"

	cityURL = "https://github.com/P3TERX/GeoLite.mmdb/releases/latest/download/GeoLite2-City.mmdb"
	asnURL  = "https://github.com/P3TERX/GeoLite.mmdb/releases/latest/download/GeoLite2-ASN.mmdb"

	downloadTimeout = 5 * time.Minute
)

// Result is the geo data resolved for an IP. All fields may be empty if the IP
// is private, malformed, or the underlying database has no record for it.
type Result struct {
	CountryCode string // ISO 3166-1 alpha-2, e.g. "JP"
	City        string // English name, e.g. "Tokyo"
	ASN         string // autonomous system org name, e.g. "DigitalOcean, LLC"
}

// IsZero reports whether no fields were populated.
func (r Result) IsZero() bool { return r.CountryCode == "" && r.City == "" && r.ASN == "" }

// Service holds the open mmdb readers. It is safe for concurrent Lookup calls;
// Refresh swaps the readers under a write lock.
type Service struct {
	dataDir string
	client  *http.Client

	mu     sync.RWMutex
	city   *maxminddb.Reader
	asn    *maxminddb.Reader
	closed bool
}

// New opens the mmdb files in dataDir, downloading any that are missing.
// A missing or corrupt file is logged but does not fail New — the service
// stays operational and Lookup just returns empty results until Refresh succeeds.
func New(dataDir string) (*Service, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("geoip: create data dir: %w", err)
	}
	s := &Service{
		dataDir: dataDir,
		client:  &http.Client{Timeout: downloadTimeout},
	}
	s.openOrFetch(cityFileName, cityURL, &s.city)
	s.openOrFetch(asnFileName, asnURL, &s.asn)
	return s, nil
}

// Lookup resolves an IP. Private, loopback, and malformed addresses return a
// zero Result with no error.
func (s *Service) Lookup(ipStr string) (Result, error) {
	if s == nil {
		return Result{}, nil
	}
	ip := net.ParseIP(ipStr)
	if ip == nil || !IsPublicIP(ip) {
		return Result{}, nil
	}

	s.mu.RLock()
	cityReader, asnReader := s.city, s.asn
	s.mu.RUnlock()

	var out Result

	if cityReader != nil {
		var rec struct {
			Country struct {
				ISOCode string `maxminddb:"iso_code"`
			} `maxminddb:"country"`
			City struct {
				Names map[string]string `maxminddb:"names"`
			} `maxminddb:"city"`
		}
		if err := cityReader.Lookup(ip, &rec); err == nil {
			out.CountryCode = rec.Country.ISOCode
			if name, ok := rec.City.Names["en"]; ok {
				out.City = name
			}
		}
	}

	if asnReader != nil {
		var rec struct {
			Org string `maxminddb:"autonomous_system_organization"`
		}
		if err := asnReader.Lookup(ip, &rec); err == nil {
			out.ASN = rec.Org
		}
	}

	return out, nil
}

// Refresh re-downloads both .mmdb files and atomically swaps the live readers.
// Errors per file are logged; partial success is fine.
func (s *Service) Refresh() {
	s.refreshOne(cityFileName, cityURL, func(r *maxminddb.Reader) {
		s.mu.Lock()
		old := s.city
		s.city = r
		s.mu.Unlock()
		if old != nil {
			_ = old.Close()
		}
	})
	s.refreshOne(asnFileName, asnURL, func(r *maxminddb.Reader) {
		s.mu.Lock()
		old := s.asn
		s.asn = r
		s.mu.Unlock()
		if old != nil {
			_ = old.Close()
		}
	})
}

// Close releases the open mmdb files.
func (s *Service) Close() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	if s.city != nil {
		_ = s.city.Close()
		s.city = nil
	}
	if s.asn != nil {
		_ = s.asn.Close()
		s.asn = nil
	}
}

// openOrFetch tries to open an existing file; if missing or corrupt, downloads
// it. Used during New() — failures are logged but non-fatal.
func (s *Service) openOrFetch(name, url string, dst **maxminddb.Reader) {
	path := filepath.Join(s.dataDir, name)
	if r, err := maxminddb.Open(path); err == nil {
		*dst = r
		return
	}
	log.Printf("geoip: %s missing, downloading from %s", name, url)
	if err := s.download(url, path); err != nil {
		log.Printf("geoip: download %s failed: %v", name, err)
		return
	}
	r, err := maxminddb.Open(path)
	if err != nil {
		log.Printf("geoip: open %s after download failed: %v", name, err)
		return
	}
	*dst = r
}

func (s *Service) refreshOne(name, url string, swap func(*maxminddb.Reader)) {
	path := filepath.Join(s.dataDir, name)
	if err := s.download(url, path); err != nil {
		log.Printf("geoip: refresh %s failed: %v", name, err)
		return
	}
	r, err := maxminddb.Open(path)
	if err != nil {
		log.Printf("geoip: reopen %s after refresh failed: %v", name, err)
		return
	}
	swap(r)
	log.Printf("geoip: refreshed %s", name)
}

// download writes url to dest atomically (download to dest+".tmp" then rename).
func (s *Service) download(url, dest string) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	tmp := dest + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, dest)
}

// IsPublicIP reports whether ip is a routable public address — i.e. not
// loopback, link-local, multicast, unspecified, or RFC1918 private.
//
// Used by GeoIP to skip lookups it can't answer, and by the agent WebSocket
// handler to decide whether the connection's source IP is trustworthy as the
// node's "real" IP.
func IsPublicIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsInterfaceLocalMulticast() ||
		ip.IsMulticast() || ip.IsPrivate() {
		return false
	}
	return true
}

// ErrNotConfigured is returned by callers that wrap a nil *Service.
var ErrNotConfigured = errors.New("geoip: service not configured")
