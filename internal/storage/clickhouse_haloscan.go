package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// ============================================================================
// Insert rows
// ============================================================================

type HaloscanOverviewRow struct {
	ProjectID           string
	Domain              string
	VisibilityIndex     float64
	TotalKeywordCount   int64
	Top10Positions      int64
	TrafficRank         int64
	KeywordCountRank    int64
	VisibilityHistory   []byte // JSON-encoded array of {date, value}
	PositionsBreakdown  []byte // JSON-encoded array of {bucket, count}
	RawMetrics          []byte // JSON-encoded raw metrics.stats
}

func (s *Store) InsertHaloscanOverview(ctx context.Context, row HaloscanOverviewRow) error {
	now := time.Now()
	q := `
		INSERT INTO crawlobserver.haloscan_overview (
			project_id, domain, visibility_index,
			total_keyword_count, top_10_positions,
			traffic_rank, keyword_count_rank,
			visibility_history, positions_breakdown, raw_metrics, fetched_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	return s.conn.Exec(ctx, q,
		row.ProjectID, row.Domain, row.VisibilityIndex,
		row.TotalKeywordCount, row.Top10Positions,
		row.TrafficRank, row.KeywordCountRank,
		string(row.VisibilityHistory), string(row.PositionsBreakdown), string(row.RawMetrics),
		now,
	)
}

type HaloscanPositionRow struct {
	ProjectID     string
	Domain        string
	Keyword       string
	URL           string
	Position      uint16
	Traffic       float64
	Volume        float64
	CPC           float64
	Competition   float64
	Kvi           float64
	WordCount     uint16
	SiInfo        bool
	SiNav         bool
	SiTrans       bool
	SiComm        bool
	SiLocal       bool
	SiBrand       bool
	PageFirstSeen time.Time
	LastScrap     string
}

func (s *Store) InsertHaloscanPositions(ctx context.Context, projectID, domain string, rows []HaloscanPositionRow) error {
	if len(rows) == 0 {
		return nil
	}
	batch, err := s.conn.PrepareBatch(ctx, `
		INSERT INTO crawlobserver.haloscan_positions (
			project_id, domain, keyword, url, position,
			traffic, volume, cpc, competition, kvi, word_count,
			si_info, si_nav, si_trans, si_comm, si_local, si_brand,
			page_first_seen, last_scrap, fetched_at
		)`)
	if err != nil {
		return fmt.Errorf("preparing haloscan_positions batch: %w", err)
	}
	now := time.Now()
	for _, r := range rows {
		pid := r.ProjectID
		if pid == "" {
			pid = projectID
		}
		dom := r.Domain
		if dom == "" {
			dom = domain
		}
		if err := batch.Append(
			pid, dom, r.Keyword, r.URL, r.Position,
			r.Traffic, r.Volume, r.CPC, r.Competition, r.Kvi, r.WordCount,
			r.SiInfo, r.SiNav, r.SiTrans, r.SiComm, r.SiLocal, r.SiBrand,
			r.PageFirstSeen, r.LastScrap, now,
		); err != nil {
			return fmt.Errorf("appending haloscan_positions row: %w", err)
		}
	}
	return batch.Send()
}

type HaloscanCompetitorRow struct {
	ProjectID         string
	Domain            string
	CompetitorDomain  string
	VisibilityIndex   float64
	Keywords          int64
	Positions         int64
	CommonKeywords    int64
	MissedKeywords    int64
	Bested            int64
	ExclusiveKeywords int64
	TotalTraffic      int64
	Rank              uint16
}

func (s *Store) InsertHaloscanCompetitors(ctx context.Context, rows []HaloscanCompetitorRow) error {
	if len(rows) == 0 {
		return nil
	}
	batch, err := s.conn.PrepareBatch(ctx, `
		INSERT INTO crawlobserver.haloscan_competitors (
			project_id, domain, competitor_domain, visibility_index,
			keywords, positions, common_keywords, missed_keywords,
			bested, exclusive_keywords, total_traffic, rank, fetched_at
		)`)
	if err != nil {
		return fmt.Errorf("preparing haloscan_competitors batch: %w", err)
	}
	now := time.Now()
	for _, r := range rows {
		if err := batch.Append(
			r.ProjectID, r.Domain, r.CompetitorDomain, r.VisibilityIndex,
			r.Keywords, r.Positions, r.CommonKeywords, r.MissedKeywords,
			r.Bested, r.ExclusiveKeywords, r.TotalTraffic, r.Rank, now,
		); err != nil {
			return fmt.Errorf("appending haloscan_competitors row: %w", err)
		}
	}
	return batch.Send()
}

type HaloscanVisibilityTrendRow struct {
	ProjectID       string
	SeriesDomain    string
	AggDate         time.Time
	VisibilityIndex float64
	SeriesType      string
}

func (s *Store) InsertHaloscanVisibilityTrends(ctx context.Context, rows []HaloscanVisibilityTrendRow) error {
	if len(rows) == 0 {
		return nil
	}
	batch, err := s.conn.PrepareBatch(ctx, `
		INSERT INTO crawlobserver.haloscan_visibility_trends (
			project_id, series_domain, agg_date, visibility_index, series_type, fetched_at
		)`)
	if err != nil {
		return fmt.Errorf("preparing haloscan_visibility_trends batch: %w", err)
	}
	now := time.Now()
	for _, r := range rows {
		if err := batch.Append(
			r.ProjectID, r.SeriesDomain, r.AggDate, r.VisibilityIndex, r.SeriesType, now,
		); err != nil {
			return fmt.Errorf("appending haloscan_visibility_trends row: %w", err)
		}
	}
	return batch.Send()
}

type HaloscanKeywordsDiffRow struct {
	ProjectID                string
	Domain                   string
	Mode                     string // missing | bested | besting | exclusive
	Keyword                  string
	Volume                   float64
	Kvi                      float64
	CPC                      float64
	BestReferencePosition    float64
	BestReferenceURL         string
	BestCompetitorPosition   float64
	BestCompetitorURL        string
	BestCompetitorDomain     string
	CompetitorsPositions     int64
	UniqueCompetitorsCount   int64
	SiInfo                   bool
	SiNav                    bool
	SiTrans                  bool
	SiComm                   bool
	SiLocal                  bool
	SiBrand                  bool
	WordCount                uint16
}

func (s *Store) InsertHaloscanKeywordsDiff(ctx context.Context, rows []HaloscanKeywordsDiffRow) error {
	if len(rows) == 0 {
		return nil
	}
	batch, err := s.conn.PrepareBatch(ctx, `
		INSERT INTO crawlobserver.haloscan_keywords_diff (
			project_id, domain, mode, keyword, volume, kvi, cpc,
			best_reference_position, best_reference_url,
			best_competitor_position, best_competitor_url, best_competitor_domain,
			competitors_positions, unique_competitors_count,
			si_info, si_nav, si_trans, si_comm, si_local, si_brand,
			word_count, fetched_at
		)`)
	if err != nil {
		return fmt.Errorf("preparing haloscan_keywords_diff batch: %w", err)
	}
	now := time.Now()
	for _, r := range rows {
		if err := batch.Append(
			r.ProjectID, r.Domain, r.Mode, r.Keyword, r.Volume, r.Kvi, r.CPC,
			r.BestReferencePosition, r.BestReferenceURL,
			r.BestCompetitorPosition, r.BestCompetitorURL, r.BestCompetitorDomain,
			r.CompetitorsPositions, r.UniqueCompetitorsCount,
			r.SiInfo, r.SiNav, r.SiTrans, r.SiComm, r.SiLocal, r.SiBrand,
			r.WordCount, now,
		); err != nil {
			return fmt.Errorf("appending haloscan_keywords_diff row: %w", err)
		}
	}
	return batch.Send()
}

// DeleteHaloscanProjectData removes all Haloscan rows for a project before a fresh sync.
func (s *Store) DeleteHaloscanProjectData(ctx context.Context, projectID string) error {
	for _, t := range []string{
		"haloscan_overview",
		"haloscan_positions",
		"haloscan_competitors",
		"haloscan_visibility_trends",
		"haloscan_keywords_diff",
	} {
		q := fmt.Sprintf("ALTER TABLE crawlobserver.%s DELETE WHERE project_id = ?", t)
		if err := s.conn.Exec(ctx, q, projectID); err != nil {
			return fmt.Errorf("deleting %s for %s: %w", t, projectID, err)
		}
	}
	return nil
}

// ============================================================================
// Read queries (used by HTTP handlers)
// ============================================================================

type HaloscanOverview struct {
	Domain              string          `json:"domain"`
	VisibilityIndex     float64         `json:"visibility_index"`
	TotalKeywordCount   int64           `json:"total_keyword_count"`
	Top10Positions      int64           `json:"top_10_positions"`
	TrafficRank         int64           `json:"traffic_rank"`
	KeywordCountRank    int64           `json:"keyword_count_rank"`
	VisibilityHistory   json.RawMessage `json:"visibility_history"`
	PositionsBreakdown  json.RawMessage `json:"positions_breakdown"`
	FetchedAt           time.Time       `json:"fetched_at"`
}

func (s *Store) GetHaloscanOverview(ctx context.Context, projectID string) (*HaloscanOverview, error) {
	var (
		o          HaloscanOverview
		visHistory string
		posBreak   string
	)
	row := s.conn.QueryRow(ctx, `
		SELECT domain, visibility_index, total_keyword_count, top_10_positions,
		       traffic_rank, keyword_count_rank,
		       visibility_history, positions_breakdown, fetched_at
		FROM crawlobserver.haloscan_overview FINAL
		WHERE project_id = ?
		LIMIT 1`, projectID)
	if err := row.Scan(
		&o.Domain, &o.VisibilityIndex, &o.TotalKeywordCount, &o.Top10Positions,
		&o.TrafficRank, &o.KeywordCountRank,
		&visHistory, &posBreak, &o.FetchedAt,
	); err != nil {
		if isNoRows(err) {
			return nil, nil
		}
		return nil, err
	}
	if visHistory != "" {
		o.VisibilityHistory = json.RawMessage(visHistory)
	}
	if posBreak != "" {
		o.PositionsBreakdown = json.RawMessage(posBreak)
	}
	return &o, nil
}

type HaloscanPositionOut struct {
	Keyword     string  `json:"keyword"`
	URL         string  `json:"url"`
	Position    uint16  `json:"position"`
	Traffic     float64 `json:"traffic"`
	Volume      float64 `json:"volume"`
	CPC         float64 `json:"cpc"`
	Kvi         float64 `json:"kvi"`
	WordCount   uint16  `json:"word_count"`
	SiInfo      bool    `json:"si_info"`
	SiNav       bool    `json:"si_nav"`
	SiTrans     bool    `json:"si_trans"`
	SiComm      bool    `json:"si_comm"`
	SiLocal     bool    `json:"si_local"`
	SiBrand     bool    `json:"si_brand"`
}

func (s *Store) ListHaloscanPositions(ctx context.Context, projectID string, positionMax uint16, limit int) ([]HaloscanPositionOut, error) {
	if limit <= 0 {
		limit = 5000
	}
	if positionMax == 0 {
		positionMax = 100
	}
	rows, err := s.conn.Query(ctx, `
		SELECT keyword, url, position, traffic, volume, cpc, kvi, word_count,
		       si_info, si_nav, si_trans, si_comm, si_local, si_brand
		FROM crawlobserver.haloscan_positions FINAL
		WHERE project_id = ? AND position <= ?
		ORDER BY traffic DESC, position ASC
		LIMIT ?`, projectID, positionMax, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []HaloscanPositionOut
	for rows.Next() {
		var r HaloscanPositionOut
		if err := rows.Scan(
			&r.Keyword, &r.URL, &r.Position, &r.Traffic, &r.Volume, &r.CPC, &r.Kvi, &r.WordCount,
			&r.SiInfo, &r.SiNav, &r.SiTrans, &r.SiComm, &r.SiLocal, &r.SiBrand,
		); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

type HaloscanCompetitorOut struct {
	CompetitorDomain  string  `json:"competitor_domain"`
	VisibilityIndex   float64 `json:"visibility_index"`
	Keywords          int64   `json:"keywords"`
	Positions         int64   `json:"positions"`
	CommonKeywords    int64   `json:"common_keywords"`
	MissedKeywords    int64   `json:"missed_keywords"`
	Bested            int64   `json:"bested"`
	ExclusiveKeywords int64   `json:"exclusive_keywords"`
	TotalTraffic      int64   `json:"total_traffic"`
	Rank              uint16  `json:"rank"`
}

func (s *Store) ListHaloscanCompetitors(ctx context.Context, projectID string, limit int) ([]HaloscanCompetitorOut, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.conn.Query(ctx, `
		SELECT competitor_domain, visibility_index, keywords, positions,
		       common_keywords, missed_keywords, bested, exclusive_keywords,
		       total_traffic, rank
		FROM crawlobserver.haloscan_competitors FINAL
		WHERE project_id = ?
		ORDER BY rank ASC, visibility_index DESC
		LIMIT ?`, projectID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []HaloscanCompetitorOut
	for rows.Next() {
		var r HaloscanCompetitorOut
		if err := rows.Scan(
			&r.CompetitorDomain, &r.VisibilityIndex, &r.Keywords, &r.Positions,
			&r.CommonKeywords, &r.MissedKeywords, &r.Bested, &r.ExclusiveKeywords,
			&r.TotalTraffic, &r.Rank,
		); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

type HaloscanTrendPoint struct {
	SeriesDomain    string    `json:"series_domain"`
	AggDate         time.Time `json:"agg_date"`
	VisibilityIndex float64   `json:"visibility_index"`
	SeriesType      string    `json:"series_type"`
}

func (s *Store) ListHaloscanTrends(ctx context.Context, projectID string) ([]HaloscanTrendPoint, error) {
	rows, err := s.conn.Query(ctx, `
		SELECT series_domain, agg_date, visibility_index, series_type
		FROM crawlobserver.haloscan_visibility_trends FINAL
		WHERE project_id = ?
		ORDER BY series_domain, agg_date`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []HaloscanTrendPoint
	for rows.Next() {
		var r HaloscanTrendPoint
		if err := rows.Scan(&r.SeriesDomain, &r.AggDate, &r.VisibilityIndex, &r.SeriesType); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

type HaloscanKeywordsDiffOut struct {
	Mode                   string  `json:"mode"`
	Keyword                string  `json:"keyword"`
	Volume                 float64 `json:"volume"`
	Kvi                    float64 `json:"kvi"`
	CPC                    float64 `json:"cpc"`
	BestReferencePosition  float64 `json:"best_reference_position"`
	BestReferenceURL       string  `json:"best_reference_url"`
	BestCompetitorPosition float64 `json:"best_competitor_position"`
	BestCompetitorURL      string  `json:"best_competitor_url"`
	BestCompetitorDomain   string  `json:"best_competitor_domain"`
	WordCount              uint16  `json:"word_count"`
}

func (s *Store) ListHaloscanKeywordsDiff(ctx context.Context, projectID, mode string, limit int) ([]HaloscanKeywordsDiffOut, error) {
	if limit <= 0 {
		limit = 500
	}
	rows, err := s.conn.Query(ctx, `
		SELECT mode, keyword, volume, kvi, cpc,
		       best_reference_position, best_reference_url,
		       best_competitor_position, best_competitor_url, best_competitor_domain,
		       word_count
		FROM crawlobserver.haloscan_keywords_diff FINAL
		WHERE project_id = ? AND mode = ?
		ORDER BY volume DESC
		LIMIT ?`, projectID, mode, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []HaloscanKeywordsDiffOut
	for rows.Next() {
		var r HaloscanKeywordsDiffOut
		if err := rows.Scan(
			&r.Mode, &r.Keyword, &r.Volume, &r.Kvi, &r.CPC,
			&r.BestReferencePosition, &r.BestReferenceURL,
			&r.BestCompetitorPosition, &r.BestCompetitorURL, &r.BestCompetitorDomain,
			&r.WordCount,
		); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// HasHaloscanData reports whether any Haloscan overview row exists for the project.
func (s *Store) HasHaloscanData(ctx context.Context, projectID string) (bool, error) {
	var n uint64
	err := s.conn.QueryRow(ctx,
		`SELECT count() FROM crawlobserver.haloscan_overview WHERE project_id = ? LIMIT 1`,
		projectID).Scan(&n)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func isNoRows(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "no rows")
}
