package service

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/James-Wolfley/steam-achievement-tracker/config"
	"github.com/James-Wolfley/steam-achievement-tracker/db"
	"github.com/James-Wolfley/steam-achievement-tracker/steamapi"
)

// RefreshStats reports what happened during a refresh run.
type RefreshStats struct {
	Owned         int   // total owned games returned by Steam (stable)
	Queued        int   // enqueued after TTL cache check
	Checked       int64 // processed AND had a non-empty schema
	Updated       int64 // snapshots inserted (hash changed)
	Skipped       int64 // unchanged vs latest snapshot (hash equal)
	SkippedCached int   // skipped at queue time due to TTL cache (no HTTP call)
	Snapshots     int64 // kept for compatibility; equals Updated
}

// RefreshUserConcurrent runs a refresh with a bounded worker pool, using a short-lived
// TTL cache for "no-achievement" games to avoid unnecessary Steam calls.
// 'workers' ~3–5 is recommended.
func RefreshUserConcurrent(ctx context.Context, repo db.Repo, client *steamapi.Client, steamid string, workers int) (RefreshStats, error) {
	if workers <= 0 {
		workers = 1
	}

	owned, err := client.GetOwnedGames(ctx, steamid)
	if err != nil {
		return RefreshStats{}, err
	}
	stats := RefreshStats{Owned: len(owned)}
	if len(owned) == 0 {
		return stats, nil
	}

	ttl := config.SchemaTTL()
	now := time.Now().UTC()

	type job struct{ g steamapi.OwnedGame }
	jobs := make(chan job, len(owned))
	errs := make(chan error, workers)

	// Queue phase: skip known-zero-achievement games if TTL is still fresh.
	queued := 0
enqueue:
	for _, g := range owned {
		select {
		case <-ctx.Done():
			break enqueue
		default:
		}
		achCount, checkedAt, cacheErr := repo.GetGameSchemaCache(ctx, g.AppID)
		if cacheErr == nil && achCount != nil && *achCount == 0 && checkedAt != nil && now.Sub(*checkedAt) < ttl {
			stats.SkippedCached++
			continue
		}
		jobs <- job{g: g}
		queued++
	}
	close(jobs)
	stats.Queued = queued

	// Workers
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				g := j.g

				// 1) Fetch schema
				defs, gameName, err := client.GetSchemaForGame(ctx, g.AppID)
				// Update cache timestamp regardless (do NOT force count to 0 on errors)
				if err != nil {
					_ = repo.UpdateGameSchemaCache(ctx, g.AppID,
						func() int {
							if ach, _, e := repo.GetGameSchemaCache(ctx, g.AppID); e == nil && ach != nil {
								return *ach
							}
							return 0
						}(),
						now,
					)
					continue
				}
				// Set cache with fresh count
				_ = repo.UpdateGameSchemaCache(ctx, g.AppID, len(defs), now)

				// 2) If no achievements, skip further work
				if len(defs) == 0 {
					continue
				}

				// 3) Upsert game + catalog
				if err := repo.UpsertGame(ctx, db.Game{AppID: g.AppID, Name: firstNonEmpty(gameName, g.Name)}); err != nil {
					select {
					case errs <- err:
					default:
					}
					return
				}
				achDefs := make([]db.AchievementDef, 0, len(defs))
				for _, d := range defs {
					achDefs = append(achDefs, db.AchievementDef{
						AppID:   g.AppID,
						APIName: d.APIName,
						Name:    d.Name,
						Descr:   d.Descr,
					})
				}
				if err := repo.UpsertAchievementDefs(ctx, achDefs); err != nil {
					select {
					case errs <- err:
					default:
					}
					return
				}

				// 4) Player states (private/empty allowed)
				states, _ := client.GetPlayerAchievements(ctx, steamid, g.AppID)

				// Build achieved map from schema (default false) + states
				achievedMap := make(map[string]bool, len(defs))
				for _, d := range defs {
					achievedMap[d.APIName] = false
				}
				for _, s := range states {
					achievedMap[s.APIName] = s.Achieved
				}

				// Precompute totals + hashes (same logic IngestOneGame will use)
				apilist := make([]string, 0, len(defs))
				for _, d := range defs {
					apilist = append(apilist, d.APIName)
				}
				totalAvail := len(apilist)
				totalDone := 0
				for _, v := range achievedMap {
					if v {
						totalDone++
					}
				}
				catHash := db.CatalogHash(g.AppID, apilist)
				items := db.BuildSnapshotAchievements(achievedMap)
				stateHash := db.StateHash(g.AppID, items)

				// Count as processed & schema-present
				atomic.AddInt64(&stats.Checked, 1)

				// 5) If unchanged vs latest snapshot → skip insert
				same, chkErr := unchangedAgainstLatest(ctx, repo, steamid, g.AppID, totalDone, totalAvail, catHash, stateHash)
				if chkErr != nil {
					select {
					case errs <- chkErr:
					default:
					}
					return
				}
				if same {
					atomic.AddInt64(&stats.Skipped, 1)
					continue
				}

				// 6) Insert snapshot (+ per-snapshot achievements) atomically
				if _, err := IngestOneGame(ctx, repo, steamid, g.AppID, apilist, achievedMap); err != nil {
					select {
					case errs <- err:
					default:
					}
					return
				}
				atomic.AddInt64(&stats.Updated, 1)
				atomic.AddInt64(&stats.Snapshots, 1)
			}
		}()
	}

	// Wait for workers and surface the first error (if any)
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
		return stats, nil
	case err := <-errs:
		return stats, err
	case <-ctx.Done():
		return stats, ctx.Err()
	}
}

// unchangedAgainstLatest returns true if the computed summary+hashes match the latest snapshot.
func unchangedAgainstLatest(ctx context.Context, repo db.Repo, steamid string, appid int64, totalDone, totalAvail int, catHash, stateHash string) (bool, error) {
	snaps, err := repo.GetLatestSnapshots(ctx, steamid, appid, 1)
	if err != nil {
		return false, err
	}
	if len(snaps) == 0 {
		return false, nil
	}
	prev := snaps[0]
	return prev.TotalDone == totalDone &&
		prev.TotalAvailable == totalAvail &&
		prev.CatalogHash == catHash &&
		prev.StateHash == stateHash, nil
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
