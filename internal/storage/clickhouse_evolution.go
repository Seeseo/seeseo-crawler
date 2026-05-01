package storage

import (
	"context"
	"fmt"
	"math"
	"time"
)

// SessionEvolutionPoint is the per-session metrics snapshot used by the
// project-level "Évolution" view in the UI. It groups page-side and link-side
// aggregates plus the session metadata needed to render a chronological chart.
type SessionEvolutionPoint struct {
	SessionID     string    `json:"session_id"`
	Label         string    `json:"label,omitempty"`
	StartedAt     time.Time `json:"started_at"`
	FinishedAt    time.Time `json:"finished_at"`
	Status        string    `json:"status"`
	TotalPages    uint64    `json:"total_pages"`
	ErrorCount    uint64    `json:"error_count"`
	NotFound      uint64    `json:"not_found"`
	ClientErrors  uint64    `json:"client_errors"`
	ServerErrors  uint64    `json:"server_errors"`
	Redirects     uint64    `json:"redirects"`
	AvgFetchMs    float64   `json:"avg_fetch_ms"`
	TotalLinks    uint64    `json:"total_links"`
	InternalLinks uint64    `json:"internal_links"`
	ExternalLinks uint64    `json:"external_links"`
}

// ProjectEvolution returns a chronological series of metrics, one entry per
// crawl session attached to the given project, ordered ascending by started_at.
// Aggregates are computed in 2 ClickHouse queries (one for pages, one for
// links) and joined in Go to avoid per-session N+1 round trips.
func (s *Store) ProjectEvolution(ctx context.Context, projectID string) ([]SessionEvolutionPoint, error) {
	sessions, err := s.ListSessions(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("listing sessions for project %s: %w", projectID, err)
	}
	if len(sessions) == 0 {
		return []SessionEvolutionPoint{}, nil
	}

	ids := make([]string, len(sessions))
	for i, sess := range sessions {
		ids[i] = sess.ID
	}

	pageStats, err := s.evolutionPageStats(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("page stats: %w", err)
	}
	linkStats, err := s.evolutionLinkStats(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("link stats: %w", err)
	}

	points := make([]SessionEvolutionPoint, 0, len(sessions))
	for _, sess := range sessions {
		p := SessionEvolutionPoint{
			SessionID:  sess.ID,
			Label:      sess.Label,
			StartedAt:  sess.StartedAt,
			FinishedAt: sess.FinishedAt,
			Status:     sess.Status,
		}
		if ps, ok := pageStats[sess.ID]; ok {
			p.TotalPages = ps.totalPages
			p.ErrorCount = ps.errorCount
			p.NotFound = ps.notFound
			p.ClientErrors = ps.clientErr
			p.ServerErrors = ps.serverErr
			p.Redirects = ps.redirects
			if !math.IsNaN(ps.avgFetchMs) {
				p.AvgFetchMs = ps.avgFetchMs
			}
		}
		if ls, ok := linkStats[sess.ID]; ok {
			p.TotalLinks = ls.total
			p.InternalLinks = ls.internal
			p.ExternalLinks = ls.external
		}
		points = append(points, p)
	}

	// Reverse to ascending chronological order (ListSessions returns DESC).
	for i, j := 0, len(points)-1; i < j; i, j = i+1, j-1 {
		points[i], points[j] = points[j], points[i]
	}
	return points, nil
}

type evolutionPageRow struct {
	totalPages uint64
	errorCount uint64
	notFound   uint64
	clientErr  uint64
	serverErr  uint64
	redirects  uint64
	avgFetchMs float64
}

func (s *Store) evolutionPageStats(ctx context.Context, sessionIDs []string) (map[string]evolutionPageRow, error) {
	out := make(map[string]evolutionPageRow, len(sessionIDs))
	if len(sessionIDs) == 0 {
		return out, nil
	}
	rows, err := s.conn.Query(ctx, `
		SELECT
			crawl_session_id,
			count() AS total,
			countIf(error != '') AS errors,
			countIf(status_code = 404) AS not_found,
			countIf(status_code >= 400 AND status_code < 500 AND status_code != 403 AND status_code != 429) AS client_err,
			countIf(status_code >= 500) AS server_err,
			countIf(status_code >= 300 AND status_code < 400) AS redirects,
			avg(fetch_duration_ms) AS avg_fetch_ms
		FROM crawlobserver.pages FINAL
		WHERE crawl_session_id IN ?
		GROUP BY crawl_session_id`, sessionIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var sid string
		var r evolutionPageRow
		if err := rows.Scan(&sid, &r.totalPages, &r.errorCount, &r.notFound, &r.clientErr, &r.serverErr, &r.redirects, &r.avgFetchMs); err != nil {
			return nil, err
		}
		out[sid] = r
	}
	return out, rows.Err()
}

type evolutionLinkRow struct {
	total    uint64
	internal uint64
	external uint64
}

func (s *Store) evolutionLinkStats(ctx context.Context, sessionIDs []string) (map[string]evolutionLinkRow, error) {
	out := make(map[string]evolutionLinkRow, len(sessionIDs))
	if len(sessionIDs) == 0 {
		return out, nil
	}
	rows, err := s.conn.Query(ctx, `
		SELECT
			crawl_session_id,
			count() AS total,
			countIf(is_internal = true) AS internal,
			countIf(is_internal = false) AS external
		FROM crawlobserver.links
		WHERE crawl_session_id IN ?
		GROUP BY crawl_session_id`, sessionIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var sid string
		var r evolutionLinkRow
		if err := rows.Scan(&sid, &r.total, &r.internal, &r.external); err != nil {
			return nil, err
		}
		out[sid] = r
	}
	return out, rows.Err()
}
