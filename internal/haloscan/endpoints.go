package haloscan

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// ============================================================================
// /domains/overview
// ============================================================================

type OverviewMetricsStats struct {
	SearchDate         string `json:"search_date"`
	RootDomain         string `json:"root_domain"`
	TrafficRank        int64  `json:"traffic_rank"`
	PageCountRank      int64  `json:"page_count_rank"`
	KeywordCountRank   int64  `json:"keyword_count_rank"`
	ActiveDomainCount  int64  `json:"active_domain_count"`
	ActivePageCount    int64  `json:"active_page_count"`
	DomainCount        int64  `json:"domain_count"`
	TotalKeywordCount  int64  `json:"total_keyword_count"`
	Top10Positions     int64  `json:"top_10_positions"`
	VisibilityIndex    Number `json:"visibility_index"`
}

type OverviewVisibilityPoint struct {
	AggDate         string `json:"agg_date"`
	VisibilityIndex Number `json:"visibility_index"`
}

type OverviewPositionsBucket struct {
	Bucket string `json:"bucket"`
	Count  int64  `json:"count"`
}

type OverviewResponse struct {
	Input  string   `json:"input"`
	Errors []string `json:"errors"`
	Metrics struct {
		Stats OverviewMetricsStats `json:"stats"`
	} `json:"metrics"`
	VisibilityIndexHistory struct {
		Site    string                    `json:"site"`
		Mode    string                    `json:"mode"`
		Results []OverviewVisibilityPoint `json:"results"`
	} `json:"visibility_index_history"`
	PositionsBreakdown struct {
		Site    string                    `json:"site"`
		Mode    string                    `json:"mode"`
		Results []OverviewPositionsBucket `json:"results"`
	} `json:"positions_breakdown"`
}

// DomainOverview fetches metrics + visibility index history + positions breakdown.
func (c *Client) DomainOverview(ctx context.Context, domain string, requested []string) (*OverviewResponse, *CallMeta, error) {
	if len(requested) == 0 {
		requested = []string{"metrics", "visibility_index_history", "positions_breakdown"}
	}
	body := map[string]interface{}{
		"input":          domain,
		"requested_data": requested,
	}
	data, meta, err := c.postJSON(ctx, "/domains/overview", body)
	if err != nil {
		return nil, meta, err
	}
	var out OverviewResponse
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, meta, fmt.Errorf("parsing overview: %w", err)
	}
	return &out, meta, nil
}

// ============================================================================
// /domains/positions  (paginated)
// ============================================================================

type Position struct {
	LastScrap          string `json:"last_scrap"`
	PageFirstSeenDate  string `json:"page_first_seen_date"`
	Position           int64  `json:"position"`
	Traffic            Number `json:"traffic"`
	Keyword            string `json:"keyword"`
	Allintitle         Number `json:"allintitle"`
	ResultCount        Number `json:"result_count"`
	AdsVolume          Number `json:"ads_volume"`
	CPC                Number `json:"cpc"`
	Competition        Number `json:"competition"`
	SiInfo             Flag   `json:"si_info"`
	SiNav              Flag   `json:"si_nav"`
	SiTrans            Flag   `json:"si_trans"`
	SiComm             Flag   `json:"si_comm"`
	SiLocal            Flag   `json:"si_local"`
	SiBrand            Flag   `json:"si_brand"`
	Kvi                Number `json:"kvi"`
	RedirectsTo        string `json:"redirects_to"`
	Volume             Number `json:"volume"`
	Kgr                Number `json:"kgr"`
	WordCount          int64  `json:"word_count"`
	URL                string `json:"url"`
}

type positionsResponse struct {
	Results []Position `json:"results"`
	Data    []Position `json:"data"`
}

// DomainPositions returns all SERP positions of a domain up to positionMax.
// Paginates internally with lineCount per page.
func (c *Client) DomainPositions(ctx context.Context, domain string, positionMax, lineCount int) ([]Position, []*CallMeta, error) {
	if lineCount <= 0 {
		lineCount = 10000
	}
	if positionMax <= 0 {
		positionMax = 100
	}
	var all []Position
	var metas []*CallMeta
	page := 1
	const guard = 20
	for page <= guard {
		body := map[string]interface{}{
			"input":        domain,
			"lineCount":    lineCount,
			"position_max": positionMax,
			"page":         page,
		}
		data, meta, err := c.postJSON(ctx, "/domains/positions", body)
		if meta != nil {
			metas = append(metas, meta)
		}
		if err != nil {
			return all, metas, err
		}
		var resp positionsResponse
		if err := json.Unmarshal(data, &resp); err != nil {
			return all, metas, fmt.Errorf("parsing positions page %d: %w", page, err)
		}
		batch := resp.Results
		if len(batch) == 0 {
			batch = resp.Data
		}
		if len(batch) == 0 {
			break
		}
		all = append(all, batch...)
		if len(batch) < lineCount {
			break
		}
		page++
		time.Sleep(200 * time.Millisecond)
	}
	return all, metas, nil
}

// ============================================================================
// /domains/siteCompetitors
// ============================================================================

type Competitor struct {
	Positions                   int64   `json:"positions"`
	Keywords                    int64   `json:"keywords"`
	MissedKeywords              int64   `json:"missed_keywords"`
	PositionsOnCommonKeywords   int64   `json:"positions_on_common_keywords"`
	CommonKeywords              int64   `json:"common_keywords"`
	Bested                      int64   `json:"bested"`
	ExclusiveKeywords           int64   `json:"exclusive_keywords"`
	PositionsOnExclusiveKeywords int64  `json:"positions_on_exclusive_keywords"`
	TotalTraffic                int64   `json:"total_traffic"`
	RootDomain                  string  `json:"root_domain"`
	VisibilityIndex             Number  `json:"visibility_index"`
	URL                         string  `json:"url"`
	KeywordsVsMax               float64 `json:"keywords_vs_max"`
	PosVsMax                    float64 `json:"pos_vs_max"`
}

type competitorsResponse struct {
	Results []Competitor `json:"results"`
	Data    []Competitor `json:"data"`
}

// DomainCompetitors fetches the top organic competitors of a domain.
func (c *Client) DomainCompetitors(ctx context.Context, domain string, lineCount int) ([]Competitor, *CallMeta, error) {
	if lineCount <= 0 {
		lineCount = 20
	}
	body := map[string]interface{}{
		"input":     domain,
		"lineCount": lineCount,
	}
	data, meta, err := c.postJSON(ctx, "/domains/siteCompetitors", body)
	if err != nil {
		return nil, meta, err
	}
	var resp competitorsResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, meta, fmt.Errorf("parsing competitors: %w", err)
	}
	out := resp.Results
	if len(out) == 0 {
		out = resp.Data
	}
	return out, meta, nil
}

// ============================================================================
// /domains/history/visibilityTrends  (multi-domains)
// ============================================================================

type VisibilityTrendPoint struct {
	AggDate         string `json:"agg_date"`
	VisibilityIndex Number `json:"visibility_index"`
	Type            string `json:"type"`
}

type VisibilityTrendSeries struct {
	Name string                 `json:"name"`
	Data []VisibilityTrendPoint `json:"data"`
}

type VisibilityTrendsResponse struct {
	Results []VisibilityTrendSeries `json:"results"`
}

// VisibilityTrends fetches the visibility index time series for several domains
// in a single API call, suitable for the benchmarked visibility chart.
func (c *Client) VisibilityTrends(ctx context.Context, domains []string) (*VisibilityTrendsResponse, *CallMeta, error) {
	body := map[string]interface{}{
		"input": domains,
	}
	data, meta, err := c.postJSON(ctx, "/domains/history/visibilityTrends", body)
	if err != nil {
		return nil, meta, err
	}
	var out VisibilityTrendsResponse
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, meta, fmt.Errorf("parsing visibility trends: %w", err)
	}
	return &out, meta, nil
}

// ============================================================================
// /domains/siteCompetitors/keywordsDiff  (gap analysis)
// ============================================================================

type KeywordDiff struct {
	BestReferencePosition  Number `json:"best_reference_position"`
	BestReferenceTraffic   Number `json:"best_reference_traffic"`
	BestCompetitorPosition Number `json:"best_competitor_position"`
	BestCompetitorTraffic  Number `json:"best_competitor_traffic"`
	CompetitorsPositions   int64  `json:"competitors_positions"`
	UniqueCompetitorsCount int64  `json:"unique_competitors_count"`
	Type                   string `json:"type"`
	Keyword                string `json:"keyword"`
	Volume                 Number `json:"volume"`
	Kvi                    Number `json:"kvi"`
	CPC                    Number `json:"cpc"`
	Competition            Number `json:"competition"`
	SiInfo                 Flag   `json:"si_info"`
	SiNav                  Flag   `json:"si_nav"`
	SiTrans                Flag   `json:"si_trans"`
	SiComm                 Flag   `json:"si_comm"`
	SiLocal                Flag   `json:"si_local"`
	SiBrand                Flag   `json:"si_brand"`
	WordCount              int64  `json:"word_count"`
	BestReferenceURL       string `json:"best_reference_url"`
	BestCompetitorURL      string `json:"best_competitor_url"`
	BestCompetitorDomain   string `json:"best_competitor_domain"`
}

type keywordsDiffResponse struct {
	Results []KeywordDiff `json:"results"`
	Data    []KeywordDiff `json:"data"`
}

// KeywordsDiffMode is one of "missing" / "bested" / "besting" / "exclusive".
type KeywordsDiffMode string

const (
	DiffMissing   KeywordsDiffMode = "missing"
	DiffBested    KeywordsDiffMode = "bested"
	DiffBesting   KeywordsDiffMode = "besting"
	DiffExclusive KeywordsDiffMode = "exclusive"
)

// KeywordsDiff runs the gap-analysis endpoint vs a list of competitor domains.
// Paginates internally up to maxPages.
func (c *Client) KeywordsDiff(ctx context.Context, domain string, competitors []string, mode KeywordsDiffMode, lineCount, maxPages int) ([]KeywordDiff, []*CallMeta, error) {
	if mode != DiffMissing && mode != DiffBested && mode != DiffBesting && mode != DiffExclusive {
		return nil, nil, fmt.Errorf("invalid keywords_diff mode: %s", mode)
	}
	if lineCount <= 0 {
		lineCount = 200
	}
	if maxPages <= 0 {
		maxPages = 5
	}
	var all []KeywordDiff
	var metas []*CallMeta
	for page := 1; page <= maxPages; page++ {
		body := map[string]interface{}{
			"input":       domain,
			"competitors": competitors,
			string(mode):  true,
			"lineCount":   lineCount,
			"page":        page,
		}
		data, meta, err := c.postJSON(ctx, "/domains/siteCompetitors/keywordsDiff", body)
		if meta != nil {
			metas = append(metas, meta)
		}
		if err != nil {
			return all, metas, err
		}
		var resp keywordsDiffResponse
		if err := json.Unmarshal(data, &resp); err != nil {
			return all, metas, fmt.Errorf("parsing keywords_diff page %d: %w", page, err)
		}
		batch := resp.Results
		if len(batch) == 0 {
			batch = resp.Data
		}
		if len(batch) == 0 {
			break
		}
		all = append(all, batch...)
		if len(batch) < lineCount {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	return all, metas, nil
}
