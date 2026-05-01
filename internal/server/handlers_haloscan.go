package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/SEObserver/crawlobserver/internal/applog"
	"github.com/SEObserver/crawlobserver/internal/haloscan"
	"github.com/SEObserver/crawlobserver/internal/storage"
)

// resolveHaloscanAPIKey returns the Haloscan API key from the cfg, then env,
// then the Claude Desktop config (where the haloscan-lucky MCP keeps it).
func (s *Server) resolveHaloscanAPIKey() string {
	if s.cfg.Haloscan.APIKey != "" {
		return s.cfg.Haloscan.APIKey
	}
	return haloscan.ResolveAPIKey()
}

func (s *Server) handleHaloscanStatus(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project id required")
		return
	}
	hasKey := s.resolveHaloscanAPIKey() != ""
	hasData, err := s.store.HasHaloscanData(r.Context(), projectID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("query: %v", err))
		return
	}
	writeJSON(w, map[string]interface{}{
		"connected":   hasKey,
		"has_data":    hasData,
		"key_source":  haloscanKeySource(s.cfg.Haloscan.APIKey),
	})
}

func haloscanKeySource(cfgKey string) string {
	if cfgKey != "" {
		return "config"
	}
	if v := haloscanEnvKey(); v != "" {
		return "env"
	}
	if k := haloscan.LoadAPIKeyFromClaudeConfig(); k != "" {
		return "claude_desktop_config"
	}
	return "none"
}

// haloscanEnvKey is split out so it can be mocked in tests if needed.
func haloscanEnvKey() string {
	return haloscan.ResolveAPIKey()
}

// HaloscanSyncRequest is the body of POST /api/projects/{id}/haloscan/sync.
type HaloscanSyncRequest struct {
	Domain      string `json:"domain"`
	PositionMax int    `json:"position_max"`
}

func (s *Server) handleHaloscanSync(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project id required")
		return
	}
	apiKey := s.resolveHaloscanAPIKey()
	if apiKey == "" {
		writeError(w, http.StatusBadRequest, "no Haloscan API key found (set haloscan.api_key in config.yaml, HALOSCAN_API_KEY env var, or the haloscan MCP config)")
		return
	}

	var req HaloscanSyncRequest
	_ = json.NewDecoder(r.Body).Decode(&req)

	// Fallback: use the project name as the domain (matches Régis' convention
	// where projects are named after the domain, e.g. "seeseo.fr", "kidosday").
	domain := strings.TrimSpace(req.Domain)
	if domain == "" {
		p, err := s.keyStore.GetProject(projectID)
		if err != nil {
			writeError(w, http.StatusNotFound, "project not found")
			return
		}
		domain = strings.TrimSpace(p.Name)
		domain = strings.TrimPrefix(domain, "https://")
		domain = strings.TrimPrefix(domain, "http://")
		domain = strings.TrimPrefix(domain, "sc-domain:")
		domain = strings.TrimSuffix(domain, "/")
	}
	if domain == "" {
		writeError(w, http.StatusBadRequest, "domain required (no body domain and project name unusable)")
		return
	}
	positionMax := req.PositionMax
	if positionMax <= 0 {
		positionMax = 100
	}

	go s.runHaloscanSync(projectID, domain, positionMax, apiKey)

	writeJSON(w, map[string]string{"status": "syncing", "domain": domain})
}

const haloscanCompetitorsForTrends = 4

// runHaloscanSync orchestrates the 5 endpoint calls and persists into ClickHouse.
// Runs in its own goroutine and logs progress via applog.
func (s *Server) runHaloscanSync(projectID, domain string, positionMax int, apiKey string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	client := haloscan.NewClient(apiKey, "1.0")
	applog.Infof("haloscan", "sync start project=%s domain=%s positionMax=%d", projectID, domain, positionMax)

	// Wipe previous rows for this project to keep the dataset coherent.
	if err := s.store.DeleteHaloscanProjectData(ctx, projectID); err != nil {
		applog.Errorf("haloscan", "delete previous data: %v", err)
		// Continue anyway — ReplacingMergeTree will dedupe on (project_id, ...)
	}

	// 1) Overview (metrics + visibility history + positions breakdown)
	overview, _, err := client.DomainOverview(ctx, domain, nil)
	if err != nil {
		applog.Errorf("haloscan", "overview failed: %v", err)
		return
	}
	visHistJSON, _ := json.Marshal(overview.VisibilityIndexHistory.Results)
	posBreakJSON, _ := json.Marshal(overview.PositionsBreakdown.Results)
	rawMetrics, _ := json.Marshal(overview.Metrics.Stats)
	if err := s.store.InsertHaloscanOverview(ctx, storage.HaloscanOverviewRow{
		ProjectID:          projectID,
		Domain:             domain,
		VisibilityIndex:    overview.Metrics.Stats.VisibilityIndex.Float64(),
		TotalKeywordCount:  overview.Metrics.Stats.TotalKeywordCount,
		Top10Positions:     overview.Metrics.Stats.Top10Positions,
		TrafficRank:        overview.Metrics.Stats.TrafficRank,
		KeywordCountRank:   overview.Metrics.Stats.KeywordCountRank,
		VisibilityHistory:  visHistJSON,
		PositionsBreakdown: posBreakJSON,
		RawMetrics:         rawMetrics,
	}); err != nil {
		applog.Errorf("haloscan", "insert overview: %v", err)
	}
	applog.Infof("haloscan", "overview ok visibility=%v", overview.Metrics.Stats.VisibilityIndex.Float64())

	// 2) Positions
	positions, _, err := client.DomainPositions(ctx, domain, positionMax, 10000)
	if err != nil {
		applog.Errorf("haloscan", "positions failed: %v", err)
	}
	if len(positions) > 0 {
		rows := make([]storage.HaloscanPositionRow, 0, len(positions))
		for _, p := range positions {
			rows = append(rows, storage.HaloscanPositionRow{
				ProjectID:     projectID,
				Domain:        domain,
				Keyword:       p.Keyword,
				URL:           p.URL,
				Position:      uint16(p.Position),
				Traffic:       p.Traffic.Float64(),
				Volume:        p.Volume.Float64(),
				CPC:           p.CPC.Float64(),
				Competition:   p.Competition.Float64(),
				Kvi:           p.Kvi.Float64(),
				WordCount:     uint16(p.WordCount),
				SiInfo:        p.SiInfo.True(),
				SiNav:         p.SiNav.True(),
				SiTrans:       p.SiTrans.True(),
				SiComm:        p.SiComm.True(),
				SiLocal:       p.SiLocal.True(),
				SiBrand:       p.SiBrand.True(),
				PageFirstSeen: parseHaloscanDateOrZero(p.PageFirstSeenDate),
				LastScrap:     p.LastScrap,
			})
		}
		if err := s.store.InsertHaloscanPositions(ctx, projectID, domain, rows); err != nil {
			applog.Errorf("haloscan", "insert positions: %v", err)
		}
		applog.Infof("haloscan", "positions ok count=%d", len(rows))
	}

	// 3) Competitors
	competitors, _, err := client.DomainCompetitors(ctx, domain, 10)
	if err != nil {
		applog.Errorf("haloscan", "competitors failed: %v", err)
	}
	competitorDomains := make([]string, 0, len(competitors))
	if len(competitors) > 0 {
		rows := make([]storage.HaloscanCompetitorRow, 0, len(competitors))
		for i, c := range competitors {
			rd := c.RootDomain
			if rd == "" {
				rd = c.URL
			}
			if rd != "" && len(competitorDomains) < haloscanCompetitorsForTrends {
				competitorDomains = append(competitorDomains, rd)
			}
			rows = append(rows, storage.HaloscanCompetitorRow{
				ProjectID:         projectID,
				Domain:            domain,
				CompetitorDomain:  rd,
				VisibilityIndex:   c.VisibilityIndex.Float64(),
				Keywords:          c.Keywords,
				Positions:         c.Positions,
				CommonKeywords:    c.CommonKeywords,
				MissedKeywords:    c.MissedKeywords,
				Bested:            c.Bested,
				ExclusiveKeywords: c.ExclusiveKeywords,
				TotalTraffic:      c.TotalTraffic,
				Rank:              uint16(i + 1),
			})
		}
		if err := s.store.InsertHaloscanCompetitors(ctx, rows); err != nil {
			applog.Errorf("haloscan", "insert competitors: %v", err)
		}
		applog.Infof("haloscan", "competitors ok count=%d", len(rows))
	}

	// 4) Visibility trends (domain + top 4 competitors on the same chart)
	if len(competitorDomains) > 0 {
		seriesDomains := append([]string{domain}, competitorDomains...)
		trends, _, err := client.VisibilityTrends(ctx, seriesDomains)
		if err != nil {
			applog.Errorf("haloscan", "visibility_trends failed: %v", err)
		} else if trends != nil {
			var rows []storage.HaloscanVisibilityTrendRow
			for _, series := range trends.Results {
				for _, pt := range series.Data {
					d, ok := parseHaloscanDate(pt.AggDate)
					if !ok {
						continue
					}
					rows = append(rows, storage.HaloscanVisibilityTrendRow{
						ProjectID:       projectID,
						SeriesDomain:    series.Name,
						AggDate:         d,
						VisibilityIndex: pt.VisibilityIndex.Float64(),
						SeriesType:      pt.Type,
					})
				}
			}
			if err := s.store.InsertHaloscanVisibilityTrends(ctx, rows); err != nil {
				applog.Errorf("haloscan", "insert trends: %v", err)
			}
			applog.Infof("haloscan", "trends ok points=%d series=%d", len(rows), len(trends.Results))
		}
	}

	// 5) Keywords diff: missing + bested
	if len(competitorDomains) > 0 {
		for _, mode := range []haloscan.KeywordsDiffMode{haloscan.DiffMissing, haloscan.DiffBested} {
			diffs, _, err := client.KeywordsDiff(ctx, domain, competitorDomains, mode, 200, 3)
			if err != nil {
				applog.Errorf("haloscan", "keywords_diff %s failed: %v", mode, err)
				continue
			}
			rows := make([]storage.HaloscanKeywordsDiffRow, 0, len(diffs))
			for _, d := range diffs {
				rows = append(rows, storage.HaloscanKeywordsDiffRow{
					ProjectID:                projectID,
					Domain:                   domain,
					Mode:                     string(mode),
					Keyword:                  d.Keyword,
					Volume:                   d.Volume.Float64(),
					Kvi:                      d.Kvi.Float64(),
					CPC:                      d.CPC.Float64(),
					BestReferencePosition:    d.BestReferencePosition.Float64(),
					BestReferenceURL:         d.BestReferenceURL,
					BestCompetitorPosition:   d.BestCompetitorPosition.Float64(),
					BestCompetitorURL:        d.BestCompetitorURL,
					BestCompetitorDomain:     d.BestCompetitorDomain,
					CompetitorsPositions:     d.CompetitorsPositions,
					UniqueCompetitorsCount:   d.UniqueCompetitorsCount,
					SiInfo:                   d.SiInfo.True(),
					SiNav:                    d.SiNav.True(),
					SiTrans:                  d.SiTrans.True(),
					SiComm:                   d.SiComm.True(),
					SiLocal:                  d.SiLocal.True(),
					SiBrand:                  d.SiBrand.True(),
					WordCount:                uint16(d.WordCount),
				})
			}
			if err := s.store.InsertHaloscanKeywordsDiff(ctx, rows); err != nil {
				applog.Errorf("haloscan", "insert keywords_diff %s: %v", mode, err)
			}
			applog.Infof("haloscan", "keywords_diff %s ok count=%d", mode, len(rows))
		}
	}

	applog.Infof("haloscan", "sync done project=%s", projectID)
}

// ============================================================================
// Read endpoints
// ============================================================================

func (s *Server) handleHaloscanOverview(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project id required")
		return
	}
	o, err := s.store.GetHaloscanOverview(r.Context(), projectID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if o == nil {
		writeJSON(w, map[string]interface{}{"has_data": false})
		return
	}
	writeJSON(w, o)
}

func (s *Server) handleHaloscanPositions(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project id required")
		return
	}
	limit := atoiOr(r.URL.Query().Get("limit"), 5000)
	posMax := atoiOr(r.URL.Query().Get("position_max"), 100)
	rows, err := s.store.ListHaloscanPositions(r.Context(), projectID, uint16(posMax), limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]interface{}{"positions": rows, "count": len(rows)})
}

func (s *Server) handleHaloscanCompetitors(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	rows, err := s.store.ListHaloscanCompetitors(r.Context(), projectID, atoiOr(r.URL.Query().Get("limit"), 20))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]interface{}{"competitors": rows, "count": len(rows)})
}

func (s *Server) handleHaloscanTrends(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	rows, err := s.store.ListHaloscanTrends(r.Context(), projectID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]interface{}{"trends": rows, "count": len(rows)})
}

func (s *Server) handleHaloscanGap(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	mode := r.URL.Query().Get("mode")
	if mode == "" {
		mode = "missing"
	}
	rows, err := s.store.ListHaloscanKeywordsDiff(r.Context(), projectID, mode, atoiOr(r.URL.Query().Get("limit"), 500))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]interface{}{"keywords": rows, "count": len(rows), "mode": mode})
}

// ============================================================================
// helpers
// ============================================================================

func atoiOr(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return def
	}
	return n
}

func parseHaloscanDate(s string) (time.Time, bool) {
	if s == "" || s == "NA" {
		return time.Time{}, false
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

func parseHaloscanDateOrZero(s string) time.Time {
	t, _ := parseHaloscanDate(s)
	return t
}
