package sqlite

import (
	"database/sql"
	"fmt"
)

type migrationRowQueryer interface {
	QueryRow(query string, args ...any) *sql.Row
}

func ensureCompleteRawSourceForDerivedRebuild(queryer migrationRowQueryer, reason string) error {
	var tableExists int
	if err := queryer.QueryRow(`select count(*) from sqlite_master where type = 'table' and name = 'usage_archive_event_refs'`).Scan(&tableExists); err != nil {
		return fmt.Errorf("inspect usage archive event refs table for derived rebuild: %w", err)
	}
	if tableExists == 0 {
		return nil
	}
	var hasDeletedRaw int
	err := queryer.QueryRow(`select exists (
		select 1 from usage_archive_event_refs where raw_deleted_at_ms is not null
	)`).Scan(&hasDeletedRaw)
	if err != nil {
		return fmt.Errorf("inspect deleted raw events for derived rebuild: %w", err)
	}
	if hasDeletedRaw != 0 {
		return fmt.Errorf(
			"cannot rebuild %s: historical raw usage events have been archived and deleted; derived rebuild requires complete raw usage history",
			reason,
		)
	}
	return nil
}
