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

// AutoSyncProjectData orchestrates the "no-click" data plane for free
// providers (Haloscan today). SEObserver is intentionally NOT auto-fetched
// here — it consumes paid API units, so we leave the trigger manual via
// the UI button on the SEObserver Data tab. The provider_connection is
// still auto-created elsewhere (seobserverautoconnect.SyncProject) so the
// user only has to click "Récupérer les données" once when they want the
// netlinking data for an audit.
//
// Domain corrections that used to happen here (SEObserver) are also moved
// out — they only matter at fetch time and are now done inline in
// handleProviderFetch when the user triggers the fetch manually.
//
// Safe to call repeatedly. Designed to run in the background.
func (s *Server) AutoSyncProjectData(projectID, projectName string) {
	go s.autoSyncHaloscan(projectID, projectName)
}

// upgradeSEObserverDomainIfNeeded transparently fixes the stored domain on
// the seobserver provider_connection when it was inferred from the project
// name and the real root domain differs (e.g. "Singular" →
// "singular-is-future.com"). Called by handleProviderFetch right before a
// manual fetch so the user always hits the right domain.
func (s *Server) upgradeSEObserverDomainIfNeeded(projectID string, conn *providers.ProviderConnection) {
	if conn == nil {
		return
	}
	real := s.resolveProjectDomain(projectID, conn.Domain)
	if real == "" || real == normaliseDomain(conn.Domain) {
		return
	}
	applog.Infof("seobserver", "upgrading domain for project %s: %q → %q", projectID, conn.Domain, real)
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
