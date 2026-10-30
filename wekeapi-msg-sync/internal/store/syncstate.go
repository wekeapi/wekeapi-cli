package store

import (
	"context"
	"database/sql"
	"fmt"
)

// 同步状态值。
const (
	SyncStatusIdle    = "idle"
	SyncStatusSyncing = "syncing"
	SyncStatusError   = "error"
)

// SyncState 同步状态。
type SyncState struct {
	Status     string
	LastSyncAt string
	LastError  string
	UpdatedAt  string
}

// SetSyncState 写入/更新同步状态。
func (s *Store) SetSyncState(ctx context.Context, owner, status, lastSyncAt, lastError string) error {
	now := nowString()
	var query string
	if s.dialect == MySQL {
		query = `INSERT INTO sync_state (wx_id, status, last_sync_at, last_error, updated_at)
			VALUES (?, ?, ?, ?, ?)
			ON DUPLICATE KEY UPDATE
			  status = VALUES(status),
			  last_sync_at = VALUES(last_sync_at),
			  last_error = VALUES(last_error),
			  updated_at = VALUES(updated_at)`
	} else {
		query = `INSERT INTO sync_state (wx_id, status, last_sync_at, last_error, updated_at)
			VALUES (?, ?, ?, ?, ?)
			ON CONFLICT(wx_id) DO UPDATE SET
			  status = excluded.status,
			  last_sync_at = excluded.last_sync_at,
			  last_error = excluded.last_error,
			  updated_at = excluded.updated_at`
	}
	if _, err := s.db.ExecContext(ctx, query, normalizeOwner(owner), status, lastSyncAt, lastError, now); err != nil {
		return fmt.Errorf("写入同步状态失败: %w", err)
	}
	return nil
}

// GetSyncState 读取同步状态；不存在时返回 idle 默认值。
func (s *Store) GetSyncState(ctx context.Context, owner string) (*SyncState, error) {
	var st SyncState
	err := s.db.QueryRowContext(ctx, `SELECT status, last_sync_at, COALESCE(last_error, ''), updated_at
		FROM sync_state WHERE wx_id = ?`, normalizeOwner(owner)).
		Scan(&st.Status, &st.LastSyncAt, &st.LastError, &st.UpdatedAt)
	if err == sql.ErrNoRows {
		return &SyncState{Status: SyncStatusIdle}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("读取同步状态失败: %w", err)
	}
	return &st, nil
}
