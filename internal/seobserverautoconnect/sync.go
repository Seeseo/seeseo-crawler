// Package seobserverautoconnect auto-creates SEObserver provider_connections
// for every project, using a single global API key from the SEObserverConfig
// (which falls back to the SEOBSERVER_API_KEY env var when empty).
//
// Goal: when the user provides one SEObserver key in config or env, every
// project benefits natively — no need to click "Connect" per project. Mirrors
// the behaviour of gscluckysync for GSC.
package seobserverautoconnect

import (
	"fmt"
	"os"
	"strings"

	"github.com/SEObserver/crawlobserver/internal/apikeys"
	"github.com/SEObserver/crawlobserver/internal/applog"
	"github.com/SEObserver/crawlobserver/internal/config"
	"github.com/SEObserver/crawlobserver/internal/providers"
	"github.com/google/uuid"
)

// ResolveAPIKey returns the SEObserver API key from config, then env var.
func ResolveAPIKey(cfg *config.SEObserverConfig) string {
	if cfg != nil && cfg.APIKey != "" {
		return cfg.APIKey
	}
	return os.Getenv("SEOBSERVER_API_KEY")
}

// guessDomain derives a likely domain from a project name. Régis' projects
// are usually named after the domain itself ("seeseo.fr", "Vuillermoz",
// "www.usine-online.com"). We strip schemes, www. and trailing slashes.
func guessDomain(projectName string) string {
	d := strings.ToLower(strings.TrimSpace(projectName))
	d = strings.TrimPrefix(d, "sc-domain:")
	d = strings.TrimPrefix(d, "https://")
	d = strings.TrimPrefix(d, "http://")
	d = strings.TrimPrefix(d, "www.")
	if i := strings.Index(d, "/"); i >= 0 {
		d = d[:i]
	}
	return d
}

// Result is what Sync reports back to the caller.
type Result struct {
	APIKeyFound        bool
	ProjectsScanned    int
	ProjectsConnected  int      // number of new connections created (existing ones left alone)
	NewlyConnectedIDs  []string // project IDs that just got their first SEObserver connection — caller can trigger an initial fetch on these
	Errors             []string
}

// Sync ensures every project has a SEObserver provider_connection if a global
// API key is available. Existing connections are left untouched (so manual
// per-project overrides survive). Safe to call repeatedly — idempotent.
func Sync(keyStore *apikeys.Store, cfg *config.SEObserverConfig) (*Result, error) {
	res := &Result{}
	apiKey := ResolveAPIKey(cfg)
	if apiKey == "" {
		applog.Infof("seobserver", "no global API key configured (set seobserver.api_key in config.yaml or SEOBSERVER_API_KEY env var) — auto-connect skipped")
		return res, nil
	}
	res.APIKeyFound = true
	applog.Infof("seobserver", "global API key detected (len=%d), scanning projects for auto-connect…", len(apiKey))

	projects, err := keyStore.ListProjects()
	if err != nil {
		return res, fmt.Errorf("listing projects: %w", err)
	}
	res.ProjectsScanned = len(projects)

	for _, p := range projects {
		if SyncProject(keyStore, cfg, p.ID, p.Name) {
			res.ProjectsConnected++
			res.NewlyConnectedIDs = append(res.NewlyConnectedIDs, p.ID)
		}
	}

	if res.ProjectsConnected > 0 {
		applog.Infof("seobserver", "auto-connected %d project(s) with the global SEObserver key", res.ProjectsConnected)
	}
	return res, nil
}

// SyncProject creates a SEObserver provider_connection for one project if
// missing. Returns true when a new connection was created. Domain inferred
// from project name. Idempotent.
func SyncProject(keyStore *apikeys.Store, cfg *config.SEObserverConfig, projectID, projectName string) bool {
	apiKey := ResolveAPIKey(cfg)
	if apiKey == "" {
		return false
	}
	// Skip projects that already have an explicit SEObserver connection —
	// the user may have set per-project overrides we shouldn't clobber.
	if existing, err := keyStore.GetProviderConnection(projectID, "seobserver"); err == nil && existing != nil && existing.APIKey != "" {
		return false
	}
	domain := guessDomain(projectName)
	if domain == "" {
		applog.Warnf("seobserver", "project %s (%s): empty domain after normalisation, skipping auto-connect", projectID, projectName)
		return false
	}
	conn := &providers.ProviderConnection{
		ID:        uuid.NewString(),
		ProjectID: projectID,
		Provider:  "seobserver",
		Domain:    domain,
		APIKey:    apiKey,
	}
	if err := keyStore.SaveProviderConnection(conn); err != nil {
		applog.Errorf("seobserver", "auto-connect %s (%s) failed: %v", projectID, domain, err)
		return false
	}
	applog.Infof("seobserver", "auto-connected project %s on domain %s", projectName, domain)
	return true
}
