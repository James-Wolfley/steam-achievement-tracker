
PRAGMA foreign_keys = ON;
PRAGMA journal_mode = WAL;

-- ===== core lookup =====
CREATE TABLE IF NOT EXISTS games (
  appid               INTEGER PRIMARY KEY,
  name                TEXT NOT NULL DEFAULT '',
  -- short-lived schema cache (TTL-controlled in app)
  achievements_count  INTEGER,        -- NULL = unknown
  schema_checked_at   DATETIME        -- NULL = never
);

CREATE INDEX IF NOT EXISTS idx_games_checked ON games(schema_checked_at);

-- Achievement catalog (definitions per game)
CREATE TABLE IF NOT EXISTS achievement_catalog (
  appid     INTEGER NOT NULL,
  apiname   TEXT    NOT NULL,
  name      TEXT    NOT NULL DEFAULT '',
  descr     TEXT    NOT NULL DEFAULT '',
  PRIMARY KEY (appid, apiname),
  FOREIGN KEY (appid) REFERENCES games(appid) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_achcat_appid ON achievement_catalog(appid);

-- ===== current player state (optional for fast lookups) =====
CREATE TABLE IF NOT EXISTS player_achievement_state (
  steamid     TEXT    NOT NULL,
  appid       INTEGER NOT NULL,
  apiname     TEXT    NOT NULL,
  achieved    INTEGER NOT NULL DEFAULT 0,   -- 0/1
  unlock_time DATETIME,                     -- may be NULL
  PRIMARY KEY (steamid, appid, apiname),
  FOREIGN KEY (appid, apiname) REFERENCES achievement_catalog(appid, apiname) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_pstate_player ON player_achievement_state(steamid, appid);

-- ===== snapshots =====
CREATE TABLE IF NOT EXISTS snapshots (
  id              INTEGER PRIMARY KEY AUTOINCREMENT,
  steamid         TEXT    NOT NULL,
  appid           INTEGER NOT NULL,
  total_done      INTEGER NOT NULL,
  total_available INTEGER NOT NULL,
  catalog_hash    TEXT    NOT NULL,
  state_hash      TEXT    NOT NULL,
  taken_at        DATETIME NOT NULL DEFAULT (datetime('now')),
  -- de-dupe: identical summary/hashes for same steamid+appid collapse to one row
  UNIQUE (steamid, appid, catalog_hash, state_hash),
  FOREIGN KEY (appid) REFERENCES games(appid) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_snap_user_game_time ON snapshots(steamid, appid, taken_at DESC);

CREATE TABLE IF NOT EXISTS snapshot_achievements (
  snapshot_id INTEGER NOT NULL,
  appid       INTEGER NOT NULL,
  apiname     TEXT    NOT NULL,
  achieved    INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (snapshot_id, apiname),
  FOREIGN KEY (snapshot_id) REFERENCES snapshots(id) ON DELETE CASCADE,
  FOREIGN KEY (appid, apiname) REFERENCES achievement_catalog(appid, apiname) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_snapach_app ON snapshot_achievements(appid, apiname);

-- ===== throttle gate (per steamid) =====
CREATE TABLE IF NOT EXISTS throttle_gate (
  steamid         TEXT PRIMARY KEY,
  last_refresh_at DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_throttle_last ON throttle_gate(last_refresh_at);
