package compare

import (
	"fmt"
	"time"

	"github.com/James-Wolfley/steam-achievement-tracker/db"
)

type Row struct {
	SteamID string
	AppID   int64

	// Snapshot times
	PrevTakenAt *time.Time // nil if no previous
	CurrTakenAt time.Time

	// Counts
	PrevDone, PrevTotal   int
	CurrDone, CurrTotal   int
	DeltaDone, DeltaTotal int

	// Percentages (0..100 scale)
	PrevPct, CurrPct float64
	DeltaPct         float64

	// Flags
	WasCompleted bool
	CompletedNow bool
	NewContent   bool // total increased
	Regression   bool // was 100%, now total>done

	// Diff lists (can be empty)
	Added       []string // new cheevos added to catalog
	Removed     []string // cheevos removed from catalog
	NewlyEarned []string // 0->1
	Lost        []string // 1->0 (rare)
}

// BuildRow assembles a comparison row from prev (optional), curr (required),
// and the per-snapshot achievement diffs (prev vs curr).
func BuildRow(prev *db.Snapshot, curr db.Snapshot, diff db.AchievementDiff) Row {
	var r Row
	r.SteamID = curr.SteamID
	r.AppID = curr.AppID
	r.CurrDone = curr.TotalDone
	r.CurrTotal = curr.TotalAvailable
	r.CurrTakenAt = curr.TakenAt
	r.CurrPct = pct(curr.TotalDone, curr.TotalAvailable)

	// Diff lists
	r.Added = diff.Added
	r.Removed = diff.Removed
	r.NewlyEarned = diff.NewlyEarned
	r.Lost = diff.Lost

	if prev != nil {
		r.PrevDone = prev.TotalDone
		r.PrevTotal = prev.TotalAvailable
		r.PrevTakenAt = &prev.TakenAt
		r.PrevPct = pct(prev.TotalDone, prev.TotalAvailable)

		r.DeltaDone = r.CurrDone - r.PrevDone
		r.DeltaTotal = r.CurrTotal - r.PrevTotal
		r.DeltaPct = r.CurrPct - r.PrevPct

		r.WasCompleted = prev.TotalDone == prev.TotalAvailable && prev.TotalAvailable > 0
	} else {
		// No previous snapshot
		r.PrevDone, r.PrevTotal = 0, 0
		r.PrevPct = 0
		r.DeltaDone = r.CurrDone
		r.DeltaTotal = r.CurrTotal
		r.DeltaPct = r.CurrPct
		r.WasCompleted = false
	}

	r.CompletedNow = r.CurrDone == r.CurrTotal && r.CurrTotal > 0
	r.NewContent = r.DeltaTotal > 0

	// Regression: previously 100% and now total > done (your rule)
	r.Regression = r.WasCompleted && (r.CurrTotal > r.CurrDone)

	return r
}

func pct(done, total int) float64 {
	if total <= 0 {
		return 0
	}
	return (float64(done) / float64(total)) * 100.0
}

// CSVHeader returns a sane header for export.
func CSVHeader() []string {
	return []string{
		"steamid", "appid",
		"prev_done", "prev_total", "prev_pct", "prev_taken_at",
		"curr_done", "curr_total", "curr_pct", "curr_taken_at",
		"delta_done", "delta_total", "delta_pct",
		"completed_now", "was_completed", "regression", "new_content",
		"added", "removed", "newly_earned", "lost",
	}
}

// ToCSV flattens the Row for CSV export. Lists are comma-separated.
func (r Row) ToCSV() []string {
	prevAt := ""
	if r.PrevTakenAt != nil {
		prevAt = r.PrevTakenAt.UTC().Format(time.RFC3339)
	}
	return []string{
		r.SteamID,
		fmt.Sprintf("%d", r.AppID),
		fmt.Sprintf("%d", r.PrevDone),
		fmt.Sprintf("%d", r.PrevTotal),
		fmt.Sprintf("%.4f", r.PrevPct),
		prevAt,
		fmt.Sprintf("%d", r.CurrDone),
		fmt.Sprintf("%d", r.CurrTotal),
		fmt.Sprintf("%.4f", r.CurrPct),
		r.CurrTakenAt.UTC().Format(time.RFC3339),
		fmt.Sprintf("%d", r.DeltaDone),
		fmt.Sprintf("%d", r.DeltaTotal),
		fmt.Sprintf("%.4f", r.DeltaPct),
		boolStr(r.CompletedNow),
		boolStr(r.WasCompleted),
		boolStr(r.Regression),
		boolStr(r.NewContent),
		strJoin(r.Added),
		strJoin(r.Removed),
		strJoin(r.NewlyEarned),
		strJoin(r.Lost),
	}
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func strJoin(xs []string) string {
	if len(xs) == 0 {
		return ""
	}
	// Comma-separated with no spaces (CSV writers will quote as needed)
	out := xs[0]
	for i := 1; i < len(xs); i++ {
		out += "," + xs[i]
	}
	return out
}
