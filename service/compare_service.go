package service

import (
	"context"
	"fmt"
	"io"

	"github.com/James-Wolfley/steam-achievement-tracker/compare"
	"github.com/James-Wolfley/steam-achievement-tracker/db"
)

// BuildComparisonForGame fetches the last two snapshots + per-snapshot achievements
// for (steamid, appid), computes diffs, and returns a ready-to-render row.
// ok=false means there is no "current" snapshot yet.
func BuildComparisonForGame(ctx context.Context, repo db.Repo, steamid string, appid int64) (row compare.Row, ok bool, err error) {
	// 1) get last two snapshots
	snaps, err := repo.GetLatestSnapshots(ctx, steamid, appid, 2)
	if err != nil {
		return compare.Row{}, false, err
	}
	if len(snaps) == 0 {
		return compare.Row{}, false, nil
	}

	var prevSnap *db.Snapshot
	currSnap := snaps[0]
	if len(snaps) > 1 {
		prevSnap = &snaps[1]
	}

	// 2) get per-snapshot achievements (prev,curr) and diff them
	prevAch, currAch, err := repo.GetLatestSnapshotAchievementsPair(ctx, steamid, appid)
	if err != nil {
		return compare.Row{}, false, err
	}
	diff := db.DiffSnapshotAchievements(prevAch, currAch)

	// 3) assemble the row
	row = compare.BuildRow(prevSnap, currSnap, diff)
	return row, true, nil
}

// BuildAllComparisonsForUser lists all appids with snapshots and builds rows.
// If there are no snapshots for the user yet, returns an empty slice.
func BuildAllComparisonsForUser(ctx context.Context, repo db.Repo, steamid string) ([]compare.Row, error) {
	appids, err := repo.ListAppIDsWithSnapshots(ctx, steamid)
	if err != nil {
		return nil, err
	}
	rows := make([]compare.Row, 0, len(appids))
	for _, appid := range appids {
		r, ok, err := BuildComparisonForGame(ctx, repo, steamid, appid)
		if err != nil {
			return nil, err
		}
		if ok {
			rows = append(rows, r)
		}
	}
	return rows, nil
}

// WriteCSV writes a CSV export (header + rows) to w.
func WriteCSV(w io.Writer, rows []compare.Row) error {
	header := compare.CSVHeader()
	if _, err := fmt.Fprintln(w, joinCSV(header)); err != nil {
		return err
	}
	for _, r := range rows {
		if _, err := fmt.Fprintln(w, joinCSV(r.ToCSV())); err != nil {
			return err
		}
	}
	return nil
}

// joinCSV is a minimal CSV joiner; values should already be safe/plain.
// (For robust quoting, you can swap to encoding/csv later.)
func joinCSV(cols []string) string {
	if len(cols) == 0 {
		return ""
	}
	out := cols[0]
	for i := 1; i < len(cols); i++ {
		out += "," + cols[i]
	}
	return out
}
