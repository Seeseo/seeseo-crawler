// Package gscluckysync auto-creates GSC connections in the SeeseoCrawler DB
// by reading the credentials.json file written by the mcp-gsc-lucky MCP server.
//
// Goal: when a project is created on a domain that the user already authorized
// via mcp-gsc-lucky, GSC analytics work out of the box, with no manual OAuth
// flow inside SeeseoCrawler. Designed to run silently at app startup and
// optionally on demand (e.g. after project creation).
package gscluckysync

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/SEObserver/crawlobserver/internal/apikeys"
	"github.com/SEObserver/crawlobserver/internal/applog"
	"github.com/SEObserver/crawlobserver/internal/config"
	"github.com/google/uuid"
)

// Credentials mirrors the JSON written by mcp-gsc-lucky.
type Credentials struct {
	ClientID          string   `json:"client_id"`
	ClientSecret      string   `json:"client_secret"`
	RefreshToken      string   `json:"refresh_token"`
	PrimarySiteURL    string   `json:"primary_site_url"`
	AvailableSiteURLs []string `json:"available_site_urls"`
}

// CredentialsPath returns the canonical filesystem path for the MCP creds.
func CredentialsPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home dir: %w", err)
	}
	return filepath.Join(home, ".config", "mcp-gsc-lucky", "credentials.json"), nil
}

// LoadCredentials reads the MCP credentials file. Returns (nil, nil) when
// the file does not exist — callers should treat that as a graceful no-op
// (the user has not configured mcp-gsc-lucky).
func LoadCredentials() (*Credentials, error) {
	path, err := CredentialsPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	var c Credentials
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return &c, nil
}

// normalizeDomain strips scheme, "sc-domain:" prefix, leading "www.", and
// trailing slash so we can compare project names with GSC site URLs.
func normalizeDomain(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	s = strings.TrimPrefix(s, "sc-domain:")
	if u, err := url.Parse(s); err == nil && u.Host != "" {
		s = u.Host
	}
	s = strings.TrimPrefix(s, "www.")
	s = strings.TrimSuffix(s, "/")
	return s
}

// matchProjectToSite is a loose domain matcher. A project named "Kidosday"
// matches the GSC site "sc-domain:kidosday.fr" because the normalized
// project ("kidosday") is contained in the normalized site ("kidosday.fr").
func matchProjectToSite(projectName, siteURL string) bool {
	p := normalizeDomain(projectName)
	s := normalizeDomain(siteURL)
	if p == "" || s == "" {
		return false
	}
	return p == s || strings.Contains(s, p) || strings.Contains(p, s)
}

// Result summarizes what a Sync run did.
type Result struct {
	ProjectsScanned int      `json:"projects_scanned"`
	Matched         int      `json:"matched"`
	Created         int      `json:"created"`
	Updated         int      `json:"updated"`
	Skipped         int      `json:"skipped"`
	Errors          []string `json:"errors,omitempty"`
}

// Sync scans MCP creds, matches available_site_urls against existing projects
// by domain, and upserts gsc_connections so that GSC works without manual
// OAuth in SeeseoCrawler. Idempotent: re-running with the same input is a no-op.
//
// Trigger points:
//   - app startup
//   - after a new project is created (via the same call)
//   - on-demand via an HTTP route (future addition)
func Sync(store *apikeys.Store, cfg *config.GSCConfig) (*Result, error) {
	creds, err := LoadCredentials()
	if err != nil {
		return nil, err
	}
	if creds == nil {
		applog.Info("gsc-lucky", "no MCP credentials at ~/.config/mcp-gsc-lucky/credentials.json — skipping sync")
		return &Result{}, nil
	}
	if creds.RefreshToken == "" || len(creds.AvailableSiteURLs) == 0 {
		applog.Info("gsc-lucky", "MCP credentials are empty — skipping sync")
		return &Result{}, nil
	}

	// Refresh tokens are bound to the OAuth client that issued them. If the
	// user's gsc.client_id / gsc.client_secret in config.yaml don't match the
	// MCP's, Google will reject the refresh. Warn loudly so the user can fix
	// config.yaml manually if needed.
	if cfg != nil && (cfg.ClientID != creds.ClientID || cfg.ClientSecret != creds.ClientSecret) {
		applog.Warnf("gsc-lucky",
			"gsc.client_id / gsc.client_secret in config.yaml differ from MCP — token refresh will fail until aligned")
	}

	projects, err := store.ListProjects()
	if err != nil {
		return nil, fmt.Errorf("listing projects: %w", err)
	}

	result := &Result{ProjectsScanned: len(projects)}

	for _, p := range projects {
		var match string
		for _, site := range creds.AvailableSiteURLs {
			if matchProjectToSite(p.Name, site) {
				match = site
				break
			}
		}
		if match == "" {
			continue
		}
		result.Matched++

		existing, err := store.GetGSCConnection(p.ID)
		if err == nil && existing != nil &&
			existing.RefreshToken == creds.RefreshToken &&
			existing.PropertyURL == match {
			result.Skipped++
			continue
		}

		conn := &apikeys.GSCConnection{
			ID:           uuid.NewString(),
			ProjectID:    p.ID,
			PropertyURL:  match,
			AccessToken:  "",
			RefreshToken: creds.RefreshToken,
			// Force a refresh on the very next GSC API call by setting the
			// expiry in the past. The OAuth library will then exchange the
			// refresh_token for a fresh access_token transparently.
			TokenExpiry: time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC),
		}
		if existing != nil {
			conn.ID = existing.ID
			result.Updated++
		} else {
			result.Created++
		}

		if err := store.SaveGSCConnection(conn); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("project %q: %v", p.Name, err))
			continue
		}
		applog.Infof("gsc-lucky", "synced project %q → %q", p.Name, match)
	}

	applog.Infof("gsc-lucky",
		"sync done: scanned=%d matched=%d created=%d updated=%d skipped=%d errors=%d",
		result.ProjectsScanned, result.Matched, result.Created, result.Updated, result.Skipped, len(result.Errors))
	return result, nil
}
