package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

// Store manages ClickHouse connections and operations.
type Store struct {
	conn driver.Conn
}

// Conn returns the underlying ClickHouse connection.
func (s *Store) Conn() driver.Conn {
	return s.conn
}

// NewStore creates a new ClickHouse store.
func NewStore(host string, port int, database, username, password string) (*Store, error) {
	conn, err := clickhouse.Open(&clickhouse.Options{
		Addr: []string{fmt.Sprintf("%s:%d", host, port)},
		Auth: clickhouse.Auth{
			Database: database,
			Username: username,
			Password: password,
		},
		Settings: clickhouse.Settings{
			"max_execution_time":                 60,
			"max_memory_usage":                   4000000000, // 4GB per query
			"max_bytes_before_external_group_by": 500000000,  // 500MB, then spill to disk
			"max_bytes_before_external_sort":     500000000,  // 500MB, then spill to disk
			"join_algorithm":                     "auto",     // auto-select hash or partial_merge based on memory
		},
		DialTimeout: 10 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("connecting to ClickHouse: %w", err)
	}

	if err := conn.Ping(context.Background()); err != nil {
		return nil, fmt.Errorf("pinging ClickHouse: %w", err)
	}

	return &Store{conn: conn}, nil
}

// Migrate runs all DDL migrations.
func (s *Store) Migrate(ctx context.Context) error {
	for i, m := range Migrations {
		if m.Fn != nil {
			if err := m.Fn(ctx, s.conn); err != nil {
				return fmt.Errorf("migration %d (%s): %w", i+1, m.Name, err)
			}
		} else {
			if err := s.conn.Exec(ctx, m.DDL); err != nil {
				return fmt.Errorf("migration %d (%s): %w", i+1, m.Name, err)
			}
		}
	}
	return nil
}

// Close closes the ClickHouse connection.
func (s *Store) Close() error {
	return s.conn.Close()
}

// CleanupOldTempTables drops the per-session temporary Join tables created by
// ComputePageRank / RecomputeDepths / RecomputeContentHashes when the related
// crawl_session is older than 24h (or doesn't exist anymore).
//
// Background : ces tables Join sont référencées par des `ALTER TABLE pages
// UPDATE ... = joinGet(tmp_*, ...)` mutations. ClickHouse persiste ces mutations
// dans `system.mutations` même après `is_done=1` ; lors de compactions MergeTree
// background ou d'INSERTs sur de nouvelles parts (autre crawl en parallèle), il
// re-prépare la mutation et `joinGet` plante si la tmp table a été droppée trop
// tôt → crash ClickHouse complet, autres crawls perdent leur connexion. On
// laisse donc les tables vivre jusqu'au prochain démarrage du serveur, où on
// purge celles dont la session est terminée depuis > 24h.
//
// Idempotent. Safe à appeler à chaque démarrage. Les erreurs sont loggées mais
// non bloquantes (le serveur démarre quand même).
func (s *Store) CleanupOldTempTables(ctx context.Context) error {
	rows, err := s.conn.Query(ctx, `
		SELECT name FROM system.tables
		WHERE database = 'crawlobserver'
		  AND (name LIKE 'tmp_pagerank_%'
		    OR name LIKE 'tmp_contenthash_%'
		    OR name LIKE 'tmp_depths_%'
		    OR name LIKE 'tmp_urlids_%'
		    OR name LIKE 'tmp_hops_%')
	`)
	if err != nil {
		return fmt.Errorf("listing tmp tables: %w", err)
	}
	defer rows.Close()

	var tmpTables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return fmt.Errorf("scan tmp table name: %w", err)
		}
		tmpTables = append(tmpTables, name)
	}

	if len(tmpTables) == 0 {
		return nil
	}

	// Récupère les SIDs des sessions en cours (running) ou récemment finies (< 24h)
	// — on garde leurs tmp tables intactes pour ne pas casser les mutations en cours.
	keep := make(map[string]bool)
	rows2, err := s.conn.Query(ctx, `
		SELECT toString(id) FROM crawl_sessions
		WHERE status = 'running'
		   OR finished_at > now() - INTERVAL 24 HOUR
	`)
	if err == nil {
		defer rows2.Close()
		for rows2.Next() {
			var sid string
			if err := rows2.Scan(&sid); err == nil {
				// Le format dans tmp_<type>_<sid_no_dashes> n'a pas de tirets
				keep[stripDashes(sid)] = true
			}
		}
	}

	dropped := 0
	for _, t := range tmpTables {
		// Extrait le sid no-dashes du nom (suffixe après le dernier '_')
		sidPart := ""
		for i := len(t) - 1; i >= 0; i-- {
			if t[i] == '_' {
				sidPart = t[i+1:]
				break
			}
		}
		if keep[sidPart] {
			continue
		}
		if err := s.conn.Exec(ctx, "DROP TABLE IF EXISTS crawlobserver."+t); err != nil {
			// On log mais on continue — la table peut être verrouillée par une
			// background mutation, on retentera au prochain boot.
			continue
		}
		dropped++
	}
	if dropped > 0 {
		fmt.Printf("[storage] cleaned up %d stale tmp tables (out of %d)\n", dropped, len(tmpTables))
	}
	return nil
}

func stripDashes(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] != '-' {
			out = append(out, s[i])
		}
	}
	return string(out)
}
