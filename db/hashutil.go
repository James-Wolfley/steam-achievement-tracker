package db

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strconv"
	"strings"
)

// CatalogHash computes a stable fingerprint of the catalog (set of apinames) for a game.
// We include appid in each line so the hash is unambiguous if ever reused.
func CatalogHash(appid int64, apinames []string) string {
	if len(apinames) == 0 {
		return sha256Hex("") // fingerprint empty catalog
	}
	lines := make([]string, 0, len(apinames))
	prefix := strconv.FormatInt(appid, 10) + "|"
	for _, n := range apinames {
		lines = append(lines, prefix+strings.TrimSpace(n))
	}
	sort.Strings(lines) // deterministic order
	return sha256Hex(strings.Join(lines, "\n"))
}

// StateHash fingerprints the player's per-achievement state for a game.
// Each line is "appid|apiname|0|1" (0/1 for achieved).
func StateHash(appid int64, items []struct {
	APIName  string
	Achieved bool
}) string {
	if len(items) == 0 {
		return sha256Hex("")
	}
	lines := make([]string, 0, len(items))
	prefix := strconv.FormatInt(appid, 10) + "|"
	for _, it := range items {
		ach := "0"
		if it.Achieved {
			ach = "1"
		}
		lines = append(lines, prefix+strings.TrimSpace(it.APIName)+"|"+ach)
	}
	sort.Strings(lines)
	return sha256Hex(strings.Join(lines, "\n"))
}

// BuildSnapshotAchievements converts a simple map into rows you can pass to InsertSnapshot.
func BuildSnapshotAchievements(state map[string]bool) []struct {
	APIName  string
	Achieved bool
} {
	if len(state) == 0 {
		return nil
	}
	out := make([]struct {
		APIName  string
		Achieved bool
	}, 0, len(state))
	for api, ach := range state {
		out = append(out, struct {
			APIName  string
			Achieved bool
		}{APIName: api, Achieved: ach})
	}
	// sort to keep output deterministic (useful in tests/logs)
	sort.Slice(out, func(i, j int) bool { return out[i].APIName < out[j].APIName })
	return out
}

// DiffSnapshotAchievements compares two per-snapshot achievement lists.
// - added: present in curr, absent in prev (catalog additions)
// - removed: present in prev, absent in curr (catalog removals)
// - newlyEarned: same apiname exists in both and flipped 0->1
// - lost: same apiname exists in both and flipped 1->0 (rare, but we track it)
type AchievementDiff struct {
	Added       []string
	Removed     []string
	NewlyEarned []string
	Lost        []string
}

func DiffSnapshotAchievements(prev, curr []SnapshotAchievement) AchievementDiff {
	toMap := func(in []SnapshotAchievement) map[string]bool {
		m := make(map[string]bool, len(in))
		for _, x := range in {
			m[x.APIName] = x.Achieved
		}
		return m
	}
	pm := toMap(prev)
	cm := toMap(curr)

	var added, removed, newly, lost []string

	for api := range cm {
		if _, ok := pm[api]; !ok {
			added = append(added, api)
		}
	}
	for api := range pm {
		if _, ok := cm[api]; !ok {
			removed = append(removed, api)
		}
	}
	for api, pv := range pm {
		if cv, ok := cm[api]; ok {
			if !pv && cv {
				newly = append(newly, api)
			} else if pv && !cv {
				lost = append(lost, api)
			}
		}
	}

	sort.Strings(added)
	sort.Strings(removed)
	sort.Strings(newly)
	sort.Strings(lost)

	return AchievementDiff{
		Added:       added,
		Removed:     removed,
		NewlyEarned: newly,
		Lost:        lost,
	}
}

func sha256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}
