package db

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

type sqliteRepo struct {
	db *sql.DB
}

func NewRepo(sqldb *sql.DB) Repo {
	return &sqliteRepo{db: sqldb}
}

// -------------------- Catalog & metadata --------------------

func (r *sqliteRepo) UpsertGame(ctx context.Context, g Game) error {
	const q = `
INSERT INTO games(appid, name)
VALUES(?, ?)
ON CONFLICT(appid) DO UPDATE SET
  name = excluded.name;`
	_, err := r.db.ExecContext(ctx, q, g.AppID, g.Name)
	return err
}

func (r *sqliteRepo) UpsertAchievementDefs(ctx context.Context, defs []AchievementDef) error {
	if len(defs) == 0 {
		return nil
	}
	const q = `
INSERT INTO achievement_catalog(appid, apiname, name, descr)
VALUES(?, ?, ?, ?)
ON CONFLICT(appid, apiname) DO UPDATE SET
  name  = excluded.name,
  descr = excluded.descr;`
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	stmt, err := tx.PrepareContext(ctx, q)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	defer stmt.Close()
	for _, d := range defs {
		if _, err := stmt.ExecContext(ctx, d.AppID, d.APIName, d.Name, d.Descr); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

// -------------------- Player state (current) --------------------

func (r *sqliteRepo) UpsertPlayerAchievementState(ctx context.Context, rows []PlayerAchievementState) error {
	if len(rows) == 0 {
		return nil
	}
	const q = `
INSERT INTO player_achievement_state(steamid, appid, apiname, achieved, unlock_time)
VALUES(?, ?, ?, ?, ?)
ON CONFLICT(steamid, appid, apiname) DO UPDATE SET
  achieved    = excluded.achieved,
  unlock_time = excluded.unlock_time;`
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	stmt, err := tx.PrepareContext(ctx, q)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	defer stmt.Close()
	for _, r0 := range rows {
		var ts any
		if r0.UnlockTime != nil {
			ts = r0.UnlockTime.UTC()
		} else {
			ts = nil
		}
		if _, err := stmt.ExecContext(ctx, r0.SteamID, r0.AppID, r0.APIName, boolToInt(r0.Achieved), ts); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

// -------------------- Snapshots --------------------

func (r *sqliteRepo) InsertSnapshot(ctx context.Context, in SnapshotInsert) (int64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	// 1) insert (or dedupe) snapshot
	const insSnap = `
INSERT INTO snapshots(steamid, appid, total_done, total_available, catalog_hash, state_hash, taken_at)
VALUES(?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
ON CONFLICT(steamid, appid, catalog_hash, state_hash) DO NOTHING;`
	if _, err := tx.ExecContext(ctx, insSnap, in.SteamID, in.AppID, in.TotalDone, in.TotalAvailable, in.CatalogHash, in.StateHash); err != nil {
		_ = tx.Rollback()
		return 0, err
	}

	// 2) get the id (new or existing)
	var id int64
	const selID = `
SELECT id FROM snapshots
WHERE steamid=? AND appid=? AND catalog_hash=? AND state_hash=?
ORDER BY taken_at DESC
LIMIT 1;`
	if err := tx.QueryRowContext(ctx, selID, in.SteamID, in.AppID, in.CatalogHash, in.StateHash).Scan(&id); err != nil {
		_ = tx.Rollback()
		return 0, err
	}

	// 3) upsert snapshot_achievements for this snapshot
	if len(in.Achievements) > 0 {
		const insA = `
INSERT INTO snapshot_achievements(snapshot_id, appid, apiname, achieved)
VALUES(?, ?, ?, ?)
ON CONFLICT(snapshot_id, apiname) DO UPDATE SET
  achieved = excluded.achieved;`
		stmt, err := tx.PrepareContext(ctx, insA)
		if err != nil {
			_ = tx.Rollback()
			return 0, err
		}
		defer stmt.Close()
		for _, a := range in.Achievements {
			if _, err := stmt.ExecContext(ctx, id, in.AppID, a.APIName, boolToInt(a.Achieved)); err != nil {
				_ = tx.Rollback()
				return 0, err
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return id, nil
}
func (r *sqliteRepo) GetLatestSnapshots(ctx context.Context, steamid string, appid int64, limit int) ([]Snapshot, error) {
	if limit <= 0 {
		limit = 2
	}
	const q = `
SELECT id, steamid, appid, total_done, total_available, catalog_hash, state_hash, taken_at
FROM snapshots
WHERE steamid=? AND appid=?
ORDER BY taken_at DESC, id DESC
LIMIT ?;`
	rows, err := r.db.QueryContext(ctx, q, steamid, appid, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Snapshot
	for rows.Next() {
		var s Snapshot
		var taken time.Time
		if scanErr := rows.Scan(&s.ID, &s.SteamID, &s.AppID, &s.TotalDone, &s.TotalAvailable, &s.CatalogHash, &s.StateHash, &taken); scanErr != nil {
			return nil, scanErr
		}
		s.TakenAt = taken
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *sqliteRepo) PruneSnapshots(ctx context.Context, steamid string, appid int64, keep int) (int64, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return 0, err
	}
	n, perr := r.pruneTx(ctx, tx, steamid, appid, keep)
	if perr != nil {
		_ = tx.Rollback()
		return 0, perr
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return n, nil
}

func (r *sqliteRepo) pruneTx(ctx context.Context, tx *sql.Tx, steamid string, appid int64, keep int) (int64, error) {
	if keep < 0 {
		return 0, errors.New("keep must be >= 0")
	}
	const q = `
DELETE FROM snapshots
WHERE id IN (
  SELECT id FROM snapshots
  WHERE steamid=? AND appid=?
  ORDER BY taken_at DESC, id DESC
  LIMIT -1 OFFSET ?
);`
	res, err := tx.ExecContext(ctx, q, steamid, appid, keep)
	if err != nil {
		return 0, err
	}
	aff, _ := res.RowsAffected()
	return aff, nil
}

// GetSnapshotAchievements returns all (apiname, achieved) for the given snapshot id.
func (r *sqliteRepo) GetSnapshotAchievements(ctx context.Context, snapshotID int64) ([]SnapshotAchievement, error) {
	const q = `
SELECT apiname, achieved
FROM snapshot_achievements
WHERE snapshot_id = ?
ORDER BY apiname ASC;`
	rows, err := r.db.QueryContext(ctx, q, snapshotID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []SnapshotAchievement
	for rows.Next() {
		var api string
		var achInt int
		if err := rows.Scan(&api, &achInt); err != nil {
			return nil, err
		}
		out = append(out, SnapshotAchievement{
			SnapshotID: snapshotID,
			APIName:    api,
			Achieved:   achInt == 1,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// GetLatestSnapshotAchievementsPair fetches the two most recent snapshots for (steamid, appid)
// and returns their achievement rows as (prev, curr). If only one exists, prev is empty.
func (r *sqliteRepo) GetLatestSnapshotAchievementsPair(ctx context.Context, steamid string, appid int64) (prev []SnapshotAchievement, curr []SnapshotAchievement, err error) {
	const qIDs = `
SELECT id
FROM snapshots
WHERE steamid=? AND appid=?
ORDER BY taken_at DESC, id DESC
LIMIT 2;`
	rows, err := r.db.QueryContext(ctx, qIDs, steamid, appid)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	switch len(ids) {
	case 0:
		return nil, nil, nil
	case 1:
		curr, err = r.GetSnapshotAchievements(ctx, ids[0])
		return nil, curr, err
	default:
		// ids[0] is newest, ids[1] is previous
		curr, err = r.GetSnapshotAchievements(ctx, ids[0])
		if err != nil {
			return nil, nil, err
		}
		prev, err = r.GetSnapshotAchievements(ctx, ids[1])
		if err != nil {
			return nil, nil, err
		}
		return prev, curr, nil
	}
}

func (r *sqliteRepo) ListAppIDsWithSnapshots(ctx context.Context, steamid string) ([]int64, error) {
	const q = `
SELECT DISTINCT appid
FROM snapshots
WHERE steamid = ?
ORDER BY appid ASC;`
	rows, err := r.db.QueryContext(ctx, q, steamid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *sqliteRepo) GetLastRefreshAt(ctx context.Context, steamid string) (time.Time, error) {
	const q = `SELECT last_refresh_at FROM throttle_gate WHERE steamid = ?;`
	var t time.Time
	err := r.db.QueryRowContext(ctx, q, steamid).Scan(&t)
	if err != nil {
		return time.Time{}, err
	}
	return t, nil
}

// SetLastRefreshNow upserts the current time for a steamid.
func (r *sqliteRepo) SetLastRefreshNow(ctx context.Context, steamid string, now time.Time) error {
	const q = `
INSERT INTO throttle_gate(steamid, last_refresh_at)
VALUES(?, ?)
ON CONFLICT(steamid) DO UPDATE SET
  last_refresh_at = excluded.last_refresh_at;`
	_, err := r.db.ExecContext(ctx, q, steamid, now.UTC())
	return err
}

func (r *sqliteRepo) GetGameSchemaCache(ctx context.Context, appid int64) (*int, *time.Time, error) {
	const q = `SELECT achievements_count, schema_checked_at FROM games WHERE appid=?;`
	var ach sql.NullInt64
	var ts sql.NullTime
	if err := r.db.QueryRowContext(ctx, q, appid).Scan(&ach, &ts); err != nil {
		return nil, nil, err
	}
	var achPtr *int
	var tsPtr *time.Time
	if ach.Valid {
		v := int(ach.Int64)
		achPtr = &v
	}
	if ts.Valid {
		t := ts.Time
		tsPtr = &t
	}
	return achPtr, tsPtr, nil
}

func (r *sqliteRepo) UpdateGameSchemaCache(ctx context.Context, appid int64, achCount int, checkedAt time.Time) error {
	const q = `
INSERT INTO games(appid, name, achievements_count, schema_checked_at)
VALUES(?, COALESCE((SELECT name FROM games WHERE appid=?), ''), ?, ?)
ON CONFLICT(appid) DO UPDATE SET
  achievements_count=excluded.achievements_count,
  schema_checked_at =excluded.schema_checked_at;`
	_, err := r.db.ExecContext(ctx, q, appid, appid, achCount, checkedAt.UTC())
	return err
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
