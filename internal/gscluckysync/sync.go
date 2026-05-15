// Package gscluckysync auto-creates GSC connections in the SeeseoCrawler DB
// by reading the credentials.json file written by the mcp-gsc-lucky MCP server.
//
// Goal: when a project is created on a domain that the user already authorized
// via mcp-gsc-lucky, GSC analytics work out of the box, with no manual OAuth
// flow inside SeeseoCrawler. Designed to run silently at app startup and
// optionally on demand (e.g. after project creation).
package gscluckysync

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
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

// refreshLiveSiteURLs exchanges the cached refresh_token for a short-lived
// access_token, then calls the Search Console API to list every property the
// user can access right now. The MCP credentials.json only stores the snapshot
// taken at OAuth time, so any property added afterwards (new client onboarded,
// new sc-domain verified...) is invisible to a startup sync that only reads
// the file. Refreshing live keeps gscluckysync in step with the live account.
func refreshLiveSiteURLs(ctx context.Context, creds *Credentials) ([]string, error) {
	form := url.Values{}
	form.Set("client_id", creds.ClientID)
	form.Set("client_secret", creds.ClientSecret)
	form.Set("refresh_token", creds.RefreshToken)
	form.Set("grant_type", "refresh_token")

	tokReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://oauth2.googleapis.com/token", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("token request: %w", err)
	}
	tokReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 15 * time.Second}
	tokResp, err := client.Do(tokReq)
	if err != nil {
		return nil, fmt.Errorf("token exchange: %w", err)
	}
	defer tokResp.Body.Close()
	tokBody, _ := io.ReadAll(tokResp.Body)
	if tokResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token exchange HTTP %d: %s", tokResp.StatusCode, strings.TrimSpace(string(tokBody)))
	}
	var tok struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(tokBody, &tok); err != nil {
		return nil, fmt.Errorf("parsing token response: %w", err)
	}
	if tok.AccessToken == "" {
		return nil, fmt.Errorf("token response missing access_token")
	}

	sitesReq, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"https://www.googleapis.com/webmasters/v3/sites", nil)
	if err != nil {
		return nil, fmt.Errorf("sites request: %w", err)
	}
	sitesReq.Header.Set("Authorization", "Bearer "+tok.AccessToken)

	sitesResp, err := client.Do(sitesReq)
	if err != nil {
		return nil, fmt.Errorf("sites list: %w", err)
	}
	defer sitesResp.Body.Close()
	sitesBody, _ := io.ReadAll(sitesResp.Body)
	if sitesResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("sites list HTTP %d: %s", sitesResp.StatusCode, strings.TrimSpace(string(sitesBody)))
	}
	var sitesPayload struct {
		SiteEntry []struct {
			SiteURL         string `json:"siteUrl"`
			PermissionLevel string `json:"permissionLevel"`
		} `json:"siteEntry"`
	}
	if err := json.Unmarshal(sitesBody, &sitesPayload); err != nil {
		return nil, fmt.Errorf("parsing sites response: %w", err)
	}
	out := make([]string, 0, len(sitesPayload.SiteEntry))
	for _, e := range sitesPayload.SiteEntry {
		if e.SiteURL == "" {
			continue
		}
		// siteUnverifiedUser entries cannot query analytics — skip them so we
		// don't auto-wire a connection that will 403 on every call.
		if e.PermissionLevel == "siteUnverifiedUser" {
			continue
		}
		out = append(out, e.SiteURL)
	}
	sort.Strings(out)
	return out, nil
}

// persistAvailableSiteURLs writes the refreshed site list back into the MCP
// credentials.json, preserving every other field (including future fields we
// don't model in the Credentials struct). A .bak-<timestamp> snapshot is
// written next to the file so the previous list stays recoverable.
func persistAvailableSiteURLs(path string, sites []string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading creds: %w", err)
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		return fmt.Errorf("parsing creds: %w", err)
	}
	backup := fmt.Sprintf("%s.bak-%d", path, time.Now().Unix())
	if err := os.WriteFile(backup, raw, 0o600); err != nil {
		return fmt.Errorf("writing backup %s: %w", backup, err)
	}
	obj["available_site_urls"] = sites
	obj["saved_at"] = time.Now().UTC().Format(time.RFC3339)
	out, err := json.MarshalIndent(obj, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling creds: %w", err)
	}
	if err := os.WriteFile(path, out, 0o600); err != nil {
		return fmt.Errorf("writing creds: %w", err)
	}
	return nil
}

// diffSiteLists returns sites present only in fresh (added) and only in old
// (removed). Used for the human-readable refresh log line.
func diffSiteLists(old, fresh []string) (added, removed []string) {
	in := func(list []string, s string) bool {
		for _, x := range list {
			if x == s {
				return true
			}
		}
		return false
	}
	for _, s := range fresh {
		if !in(old, s) {
			added = append(added, s)
		}
	}
	for _, s := range old {
		if !in(fresh, s) {
			removed = append(removed, s)
		}
	}
	return added, removed
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
	if creds.RefreshToken == "" {
		applog.Info("gsc-lucky", "MCP credentials missing refresh_token — skipping sync")
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

	// Refresh the site list from Google before matching. The cached list in
	// credentials.json only reflects the OAuth-time snapshot; properties
	// granted afterwards stay invisible until we re-query the live API.
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if live, err := refreshLiveSiteURLs(ctx, creds); err != nil {
		applog.Warnf("gsc-lucky", "live site refresh failed (%v) — falling back to cached list (%d sites)",
			err, len(creds.AvailableSiteURLs))
	} else {
		added, removed := diffSiteLists(creds.AvailableSiteURLs, live)
		creds.AvailableSiteURLs = live
		if path, perr := CredentialsPath(); perr == nil {
			if werr := persistAvailableSiteURLs(path, live); werr != nil {
				applog.Warnf("gsc-lucky", "persisting refreshed site list failed: %v", werr)
			}
		}
		applog.Infof("gsc-lucky", "live site refresh: %d sites (+%d new / -%d gone)",
			len(live), len(added), len(removed))
		for _, s := range added {
			applog.Infof("gsc-lucky", "  + %s", s)
		}
	}

	if len(creds.AvailableSiteURLs) == 0 {
		applog.Info("gsc-lucky", "no available sites — skipping sync")
		return &Result{}, nil
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
