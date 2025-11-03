package db

import (
	"context"
	"database/sql"
	"time"
)

// Re-export so callers can check db.ErrNoRows without importing database/sql.
var ErrNoRows = sql.ErrNoRows

// ---------- Row models (mirror your schema) ----------

type Game struct {
	AppID int64
	Name  string
}

type AchievementDef struct {
	AppID   int64
	APIName string
	Name    string
	Descr   string
}

// Player’s current per-achievement state (not snapshot).
type PlayerAchievementState struct {
	SteamID    string
	AppID      int64
	APIName    string
	Achieved   bool
	UnlockTime *time.Time // nil if unknown / never unlocked
}

type Snapshot struct {
	ID             int64
	SteamID        string
	AppID          int64
	TotalDone      int
	TotalAvailable int
	CatalogHash    string
	StateHash      string
	TakenAt        time.Time
}

type SnapshotAchievement struct {
	SnapshotID int64
	APIName    string
	Achieved   bool
}

// ---------- Inputs for snapshot insertion (clean call site) ----------

type SnapshotInsert struct {
	SteamID        string
	AppID          int64
	TotalDone      int
	TotalAvailable int
	CatalogHash    string
	StateHash      string
	Achievements   []struct {
		APIName  string
		Achieved bool
	}
}

type Repo interface {
	UpsertGame(ctx context.Context, g Game) error
	UpsertAchievementDefs(ctx context.Context, defs []AchievementDef) error
	UpsertPlayerAchievementState(ctx context.Context, rows []PlayerAchievementState) error
	InsertSnapshot(ctx context.Context, in SnapshotInsert) (int64, error)
	GetLatestSnapshots(ctx context.Context, steamid string, appid int64, limit int) ([]Snapshot, error)
	PruneSnapshots(ctx context.Context, steamid string, appid int64, keep int) (int64, error)
	GetSnapshotAchievements(ctx context.Context, snapshotID int64) ([]SnapshotAchievement, error)
	GetLatestSnapshotAchievementsPair(ctx context.Context, steamid string, appid int64) (prev []SnapshotAchievement, curr []SnapshotAchievement, err error)
	ListAppIDsWithSnapshots(ctx context.Context, steamid string) ([]int64, error)
	GetLastRefreshAt(ctx context.Context, steamid string) (time.Time, error) // ErrNoRows if none
	SetLastRefreshNow(ctx context.Context, steamid string, now time.Time) error
	GetGameSchemaCache(ctx context.Context, appid int64) (achCount *int, checkedAt *time.Time, err error)
	UpdateGameSchemaCache(ctx context.Context, appid int64, achCount int, checkedAt time.Time) error
}
