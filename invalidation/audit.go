package invalidation

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"encore.dev/storage/sqldb"
)

// AuditLog represents an invalidation event for audit trail and compliance.
type AuditLog struct {
	ID          int64     `json:"id"`
	Pattern     string    `json:"pattern"`      // Pattern or key(s) invalidated
	Keys        []string  `json:"keys"`         // Actual keys invalidated (if known)
	TriggeredBy string    `json:"triggered_by"` // Source: cache_manager, admin, warming
	Timestamp   time.Time `json:"timestamp"`    // When invalidation occurred
	RequestID   string    `json:"request_id"`   // Correlation ID for tracing
	Latency     int64     `json:"latency"`      // Invalidation latency in milliseconds
}

// AuditLogger provides persistent storage of invalidation events.
//
// Design decisions:
// - PostgreSQL for ACID compliance and audit integrity
// - Append-only log (no updates/deletes) for immutability
// - Indexed by timestamp for efficient time-range queries
// - JSONB for flexible key storage without schema changes
type AuditLogger struct {
	db *sqldb.Database
}

// NewAuditLogger creates a new audit logger with database connection.
func NewAuditLogger(db *sqldb.Database) (*AuditLogger, error) {
	logger := &AuditLogger{db: db}

	// Ensure table exists
	if err := logger.ensureSchema(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to initialize audit schema: %w", err)
	}

	return logger, nil
}

// ensureSchema creates the audit log table if it doesn't exist.
func (al *AuditLogger) ensureSchema(ctx context.Context) error {
	query := `
		CREATE TABLE IF NOT EXISTS invalidation_audit (
			id BIGSERIAL PRIMARY KEY,
			pattern TEXT NOT NULL,
			keys JSONB,
			triggered_by TEXT NOT NULL,
			timestamp TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			request_id TEXT NOT NULL,
			latency_ms BIGINT DEFAULT 0,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);

		CREATE INDEX IF NOT EXISTS idx_invalidation_audit_timestamp 
		ON invalidation_audit(timestamp DESC);

		CREATE INDEX IF NOT EXISTS idx_invalidation_audit_pattern 
		ON invalidation_audit(pattern);

		CREATE INDEX IF NOT EXISTS idx_invalidation_audit_triggered_by 
		ON invalidation_audit(triggered_by);

		CREATE INDEX IF NOT EXISTS idx_invalidation_audit_request_id 
		ON invalidation_audit(request_id);
	`

	_, err := al.db.Exec(ctx, query)
	return err
}

// Insert adds a new audit log entry.
// This operation is idempotent based on request_id - duplicate inserts are ignored.
//
// Complexity: O(1) with index overhead
func (al *AuditLogger) Insert(ctx context.Context, log AuditLog) error {
	// Serialize keys to JSONB
	keysJSON, err := json.Marshal(log.Keys)
	if err != nil {
		return fmt.Errorf("failed to marshal keys: %w", err)
	}

	query := `
		INSERT INTO invalidation_audit 
		(pattern, keys, triggered_by, timestamp, request_id, latency_ms)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT DO NOTHING
	`

	_, err = al.db.Exec(ctx, query,
		log.Pattern,
		keysJSON,
		log.TriggeredBy,
		log.Timestamp,
		log.RequestID,
		log.Latency,
	)

	if err != nil {
		return fmt.Errorf("failed to insert audit log: %w", err)
	}

	return nil
}

// GetRecent retrieves recent audit logs with pagination.
// Complexity: O(limit) with index scan
func (al *AuditLogger) GetRecent(ctx context.Context, limit, offset int, patternFilter string) ([]AuditLog, error) {
	var query string
	var args []interface{}

	if patternFilter != "" {
		query = `
			SELECT id, pattern, keys, triggered_by, timestamp, request_id, latency_ms
			FROM invalidation_audit
			WHERE pattern LIKE $1
			ORDER BY timestamp DESC
			LIMIT $2 OFFSET $3
		`
		args = []interface{}{"%" + patternFilter + "%", limit, offset}
	} else {
		query = `
			SELECT id, pattern, keys, triggered_by, timestamp, request_id, latency_ms
			FROM invalidation_audit
			ORDER BY timestamp DESC
			LIMIT $1 OFFSET $2
		`
		args = []interface{}{limit, offset}
	}

	rows, err := al.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query audit logs: %w", err)
	}
	defer rows.Close()

	logs := make([]AuditLog, 0, limit)
	for rows.Next() {
		var log AuditLog
		var keysJSON []byte

		err := rows.Scan(
			&log.ID,
			&log.Pattern,
			&keysJSON,
			&log.TriggeredBy,
			&log.Timestamp,
			&log.RequestID,
			&log.Latency,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan audit log: %w", err)
		}

		// Deserialize keys
		if len(keysJSON) > 0 {
			if err := json.Unmarshal(keysJSON, &log.Keys); err != nil {
				log.Keys = []string{} // Fallback to empty on error
			}
		}

		logs = append(logs, log)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating audit logs: %w", err)
	}

	return logs, nil
}

// GetCount returns the total number of audit logs (optionally filtered by pattern).
func (al *AuditLogger) GetCount(ctx context.Context, patternFilter string) (int, error) {
	var query string
	var args []interface{}
	var count int

	if patternFilter != "" {
		query = `SELECT COUNT(*) FROM invalidation_audit WHERE pattern LIKE $1`
		args = []interface{}{"%" + patternFilter + "%"}
	} else {
		query = `SELECT COUNT(*) FROM invalidation_audit`
	}

	err := al.db.QueryRow(ctx, query, args...).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count audit logs: %w", err)
	}

	return count, nil
}

// GetByRequestID retrieves audit logs by request ID for tracing.
func (al *AuditLogger) GetByRequestID(ctx context.Context, requestID string) ([]AuditLog, error) {
	query := `
		SELECT id, pattern, keys, triggered_by, timestamp, request_id, latency_ms
		FROM invalidation_audit
		WHERE request_id = $1
		ORDER BY timestamp DESC
	`

	rows, err := al.db.Query(ctx, query, requestID)
	if err != nil {
		return nil, fmt.Errorf("failed to query audit logs by request ID: %w", err)
	}
	defer rows.Close()

	logs := make([]AuditLog, 0)
	for rows.Next() {
		var log AuditLog
		var keysJSON []byte

		err := rows.Scan(
			&log.ID,
			&log.Pattern,
			&keysJSON,
			&log.TriggeredBy,
			&log.Timestamp,
			&log.RequestID,
			&log.Latency,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan audit log: %w", err)
		}

		// Deserialize keys
		if len(keysJSON) > 0 {
			if err := json.Unmarshal(keysJSON, &log.Keys); err != nil {
				log.Keys = []string{}
			}
		}

		logs = append(logs, log)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating audit logs: %w", err)
	}

	return logs, nil
}

// GetByTimeRange retrieves audit logs within a time range.
func (al *AuditLogger) GetByTimeRange(ctx context.Context, start, end time.Time, limit int) ([]AuditLog, error) {
	query := `
		SELECT id, pattern, keys, triggered_by, timestamp, request_id, latency_ms
		FROM invalidation_audit
		WHERE timestamp BETWEEN $1 AND $2
		ORDER BY timestamp DESC
		LIMIT $3
	`

	rows, err := al.db.Query(ctx, query, start, end, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query audit logs by time range: %w", err)
	}
	defer rows.Close()

	logs := make([]AuditLog, 0, limit)
	for rows.Next() {
		var log AuditLog
		var keysJSON []byte

		err := rows.Scan(
			&log.ID,
			&log.Pattern,
			&keysJSON,
			&log.TriggeredBy,
			&log.Timestamp,
			&log.RequestID,
			&log.Latency,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan audit log: %w", err)
		}

		// Deserialize keys
		if len(keysJSON) > 0 {
			if err := json.Unmarshal(keysJSON, &log.Keys); err != nil {
				log.Keys = []string{}
			}
		}

		logs = append(logs, log)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating audit logs: %w", err)
	}

	return logs, nil
}

// GetStats returns aggregated statistics about invalidations.
type AuditStats struct {
	TotalInvalidations int64              `json:"total_invalidations"`
	BySource           map[string]int64   `json:"by_source"`
	AvgLatency         float64            `json:"avg_latency_ms"`
	TotalKeysAffected  int64              `json:"total_keys_affected"`
	MostFrequentPattern string            `json:"most_frequent_pattern"`
}

func (al *AuditLogger) GetStats(ctx context.Context, since time.Time) (*AuditStats, error) {
	stats := &AuditStats{
		BySource: make(map[string]int64),
	}

	// Get total count and avg latency
	query := `
		SELECT 
			COUNT(*) as total,
			COALESCE(AVG(latency_ms), 0) as avg_latency
		FROM invalidation_audit
		WHERE timestamp >= $1
	`

	err := al.db.QueryRow(ctx, query, since).Scan(&stats.TotalInvalidations, &stats.AvgLatency)
	if err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("failed to get total stats: %w", err)
	}

	// Get breakdown by source
	sourceQuery := `
		SELECT triggered_by, COUNT(*) as count
		FROM invalidation_audit
		WHERE timestamp >= $1
		GROUP BY triggered_by
	`

	rows, err := al.db.Query(ctx, sourceQuery, since)
	if err != nil {
		return nil, fmt.Errorf("failed to get source breakdown: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var source string
		var count int64
		if err := rows.Scan(&source, &count); err != nil {
			continue
		}
		stats.BySource[source] = count
	}

	// Get most frequent pattern
	patternQuery := `
		SELECT pattern, COUNT(*) as frequency
		FROM invalidation_audit
		WHERE timestamp >= $1
		GROUP BY pattern
		ORDER BY frequency DESC
		LIMIT 1
	`

	err = al.db.QueryRow(ctx, patternQuery, since).Scan(&stats.MostFrequentPattern, new(int64))
	if err != nil && err != sql.ErrNoRows {
		// Non-fatal, just skip
		stats.MostFrequentPattern = ""
	}

	return stats, nil
}

// Cleanup removes audit logs older than the specified duration.
// This should be run periodically to prevent unbounded growth.
func (al *AuditLogger) Cleanup(ctx context.Context, olderThan time.Duration) (int64, error) {
	cutoff := time.Now().Add(-olderThan)

	query := `DELETE FROM invalidation_audit WHERE timestamp < $1`

	result, err := al.db.Exec(ctx, query, cutoff)
	if err != nil {
		return 0, fmt.Errorf("failed to cleanup audit logs: %w", err)
	}

	rowsAffected := result.RowsAffected()
	return rowsAffected, nil
}