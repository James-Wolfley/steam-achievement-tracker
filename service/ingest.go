package service

import (
	"context"

	"github.com/James-Wolfley/steam-achievement-tracker/db"
)

// IngestOneGame inserts a snapshot for a single (steamid, appid) using the
// provided catalog (apinames) and current player state (map apiname -> achieved).
// It computes total_done/total_available, catalog/state hashes, and persists
// snapshot_achievements atomically with the snapshot.
//
// Returns the snapshot id (existing or newly inserted, per UNIQUE dedupe).
func IngestOneGame(ctx context.Context, repo db.Repo, steamid string, appid int64, apinames []string, achieved map[string]bool) (int64, error) {
	totalAvail := len(apinames)
	totalDone := 0
	for _, v := range achieved {
		if v {
			totalDone++
		}
	}

	catHash := db.CatalogHash(appid, apinames)
	items := db.BuildSnapshotAchievements(achieved)
	stateHash := db.StateHash(appid, items)

	in := db.SnapshotInsert{
		SteamID:        steamid,
		AppID:          appid,
		TotalDone:      totalDone,
		TotalAvailable: totalAvail,
		CatalogHash:    catHash,
		StateHash:      stateHash,
		Achievements:   items,
	}
	return repo.InsertSnapshot(ctx, in)
}
