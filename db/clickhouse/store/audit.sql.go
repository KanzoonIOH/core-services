package store

import "context"

// SelectAuditLogsParams filters the audit feed. Empty string filters are
// ignored (match-all), so the same query serves the global feed and any
// single-facet drill-down (by user, menu, or action) without extra queries.
type SelectAuditLogsParams struct {
	UserID string `json:"user_id"`
	Menu   string `json:"menu"`
	Action string `json:"action"`
	Limit  int32  `json:"limit"`
	Offset int32  `json:"offset"`
}

const selectAuditLogs = `-- name: SelectAuditLogs :many
SELECT
    occurred_at,
    user_id,
    role,
    auth_method,
    action,
    menu,
    method,
    path,
    status
FROM audit_logs
WHERE (? = '' OR user_id = ?)
  AND (? = '' OR menu = ?)
  AND (? = '' OR action = ?)
ORDER BY occurred_at DESC
LIMIT ?, ?
`

func (q *Queries) SelectAuditLogs(ctx context.Context, arg SelectAuditLogsParams) ([]AuditLogView, error) {
	rows, err := q.conn.Query(ctx, selectAuditLogs,
		arg.UserID, arg.UserID,
		arg.Menu, arg.Menu,
		arg.Action, arg.Action,
		arg.Offset, arg.Limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []AuditLogView{}
	for rows.Next() {
		var i AuditLogView
		if err := rows.Scan(
			&i.OccurredAt,
			&i.UserID,
			&i.Role,
			&i.AuthMethod,
			&i.Action,
			&i.Menu,
			&i.Method,
			&i.Path,
			&i.Status,
		); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	return items, rows.Err()
}

const countAuditLogs = `-- name: CountAuditLogs :one
SELECT count(*) FROM audit_logs
WHERE (? = '' OR user_id = ?)
  AND (? = '' OR menu = ?)
  AND (? = '' OR action = ?)
`

func (q *Queries) CountAuditLogs(ctx context.Context, arg SelectAuditLogsParams) (int64, error) {
	row := q.conn.QueryRow(ctx, countAuditLogs,
		arg.UserID, arg.UserID,
		arg.Menu, arg.Menu,
		arg.Action, arg.Action,
	)
	var count uint64
	err := row.Scan(&count)
	return int64(count), err
}
