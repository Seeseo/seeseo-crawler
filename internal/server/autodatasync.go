package server

import (
	"context"
	"net/url"
	"strings"
	"time"

	"github.com/SEObserver/crawlobserver/internal/applog"
	"github.com/SEObserver/crawlobserver/internal/providers"
	"github.com/SEObserver/crawlobserver/internal/seobserverautoconnect"
)

// resolveProjectDomain tries to find the real root domain for a project,
// in order of trust:
//  1. The project's GSC connection property_url
//  2. The latest crawl session's first seed URL
//  3. The project name itself (last resort, often wrong like "Singular" vs
//     "singular-is-future.com")
//
// Returns lowercase domain without scheme, www. or trailing slash. Empty if
// nothing usable is found.
func (s *Server) resolveProjectDomain(projectID, projectName string) string {
	// 1) GSC property URL
	if conn, err := s.keyStore.GetGSCConnection(projectID); err == nil && conn != nil && conn.PropertyURL != "" {
		if d := normaliseDomain(conn.PropertyURL); d != "" {
			return d
		}
	}
	// 2) Latest crawl session seed URL
	if sessions, err := s.store.ListSessions(context.Background(), projectID); err == nil {
		for _, sess := range sessions {
			if len(sess.SeedURLs) > 0 {
				if d := normaliseDomain(sess.SeedURLs[0]); d != "" {
					return d
				}
			}
		}
	}
	// 3) Fallback: cleaned project name
	return normaliseDomain(projectName)
}

func normaliseDomain(input string) string {
	d := strings.ToLower(strings.TrimSpace(input))
	d = strings.TrimPrefix(d, "sc-domain:")
	if u, err := url.Parse(d); err == nil && u.Host != "" {
		d = u.Host
	}
	d = strings.TrimPrefix(d, "https://")
	d = strings.TrimPrefix(d, "http://")
	d = strings.TrimPrefix(d, "www.")
	if i := strings.Index(d, "/"); i >= 0 {
		d = d[:i]
	}
	return d
}

// AutoSyncProjectData orchestrates the "no-click" data plane: when called for
// a project, it ensures both SEObserver and Haloscan are populated without
// the user lifting a finger.
//
// SEObserver: triggers a background runProviderFetch for all data types if
// the project has a connection (typically auto-created via seobserverautoconnect)
// and no recent data — guarded by the existing 24h cache in runProviderFetch
// so re-running this is cheap.
//
// Haloscan: triggers runHaloscanSync if a key is available and the project
// has no Haloscan data yet. Re-runs are no-ops once data exists; the user can
// always manually re-sync from the UI.
//
// Safe to call repeatedly. Designed to run in the background.
func (s *Server) AutoSyncProjectData(projectID, projectName string) {
	go s.autoSyncSEObserver(projectID)
	go s.autoSyncHaloscan(projectID, projectName)
}

func (s *Server) autoSyncSEObserver(projectID string) {
	conn, err := s.keyStore.GetProviderConnection(projectID, "seobserver")
	if err != nil || conn == nil || conn.APIKey == "" {
		return
	}
	// If the stored domain looks wrong (was inferred from project name, not
	// the real root domain), upgrade it transparently with the resolved one
	// before fetching. This fixes auto-connections created when the project
	// name didn't match the real domain (e.g. "Singular" → "singular-is-future.com").
	if real := s.resolveProjectDomain(projectID, conn.Domain); real != "" && real != normaliseDomain(conn.Domain) {
		applog.Infof("autosync", "seobserver: upgrading domain for project %s: %q → %q", projectID, conn.Domain, real)
		conn.Domain = real
		_ = s.keyStore.SaveProviderConnection(&providers.ProviderConnection{
			ID:              conn.ID,
			ProjectID:       conn.ProjectID,
			Provider:        conn.Provider,
			Domain:          real,
			APIKey:          conn.APIKey,
			LimitBacklinks:  conn.LimitBacklinks,
			LimitRefdomains: conn.LimitRefdomains,
			LimitRankings:   conn.LimitRankings,
			LimitTopPages:   conn.LimitTopPages,
		})
	}
	key := s.providerFetchKey(projectID, "seobserver")

	s.providerFetchMu.Lock()
	if s.providerFetchStatus == nil {
		s.providerFetchStatus = make(map[string]*providerFetchStatus)
	}
	if existing := s.providerFetchStatus[key]; existing != nil && existing.Fetching {
		s.providerFetchMu.Unlock()
		return // already fetching
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.providerFetchStatus[key] = &providerFetchStatus{Fetching: true, Phase: "starting", cancel: cancel}
	s.providerFetchMu.Unlock()

	dataTypes := []string{"metrics", "backlinks", "refdomains", "rankings", "visibility", "top_pages"}
	applog.Infof("autosync", "seobserver fetch start project=%s domain=%s", projectID, conn.Domain)
	s.runProviderFetch(ctx, cancel, projectID, "seobserver", conn, dataTypes, false, key)
}

func (s *Server) autoSyncHaloscan(projectID, projectName string) {
	apiKey := s.resolveHaloscanAPIKey()
	if apiKey == "" {
		return
	}
	hasData, err := s.store.HasHaloscanData(context.Background(), projectID)
	if err != nil {
		applog.Errorf("autosync", "haloscan check has_data project=%s: %v", projectID, err)
		return
	}
	if hasData {
		// Already populated — let the user trigger re-syncs manually from the UI.
		return
	}
	domain := s.resolveProjectDomain(projectID, projectName)
	if domain == "" {
		applog.Warnf("autosync", "haloscan: empty domain for project %s (%s), skip auto-sync", projectID, projectName)
		return
	}
	applog.Infof("autosync", "haloscan sync start project=%s domain=%s", projectID, domain)
	s.runHaloscanSync(projectID, domain, 100, apiKey)
}

// AutoSyncAllProjects walks every existing project and kicks off the auto data
// sync for each. Called once at startup, after gscluckysync + seobserverautoconnect
// have had a chance to run. Each project's sync runs concurrently (bounded by
// SEObserver/Haloscan rate limits — they're sequential per provider per project).
func (s *Server) AutoSyncAllProjects() {
	projects, err := s.keyStore.ListProjects()
	if err != nil {
		applog.Errorf("autosync", "listing projects failed: %v", err)
		return
	}
	applog.Infof("autosync", "scanning %d project(s) for missing SEObserver/Haloscan data", len(projects))
	for _, p := range projects {
		s.AutoSyncProjectData(p.ID, p.Name)
		// Stagger to avoid hammering both APIs simultaneously across N projects.
		time.Sleep(500 * time.Millisecond)
	}
}


// Used by gui.go / handleCreateProject to know if we should bother with
// SEObserver auto-fetch (depends on whether a global key is configured).
func seobserverGlobalKey(s *Server) string {
	return seobserverautoconnect.ResolveAPIKey(&s.cfg.SEObserver)
}
