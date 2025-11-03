package main

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/James-Wolfley/steam-achievement-tracker/config"
	"github.com/James-Wolfley/steam-achievement-tracker/db"
	"github.com/James-Wolfley/steam-achievement-tracker/service"
	"github.com/James-Wolfley/steam-achievement-tracker/steamapi"
	"github.com/James-Wolfley/steam-achievement-tracker/views"
	"github.com/labstack/echo/v4"
)

func (app *Application) Home(c echo.Context) error {
	return render(c, http.StatusOK, views.Home())
}

// GET /api/results/:steamid
// Returns the ready-to-render comparison rows for all games with snapshots.
func (app *Application) APIResults(c echo.Context) error {
	steamid := c.Param("steamid")
	ctx := c.Request().Context()

	rows, err := service.BuildAllComparisonsForUser(ctx, app.Repo, steamid)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, rows)
}

// GET /export/:steamid.csv
// Streams a CSV with header + rows (may be header-only if no snapshots exist).
func (app *Application) ExportCSV(c echo.Context) error {
	steamid := c.Param("steamid")
	ctx := c.Request().Context()

	rows, err := service.BuildAllComparisonsForUser(ctx, app.Repo, steamid)
	if err != nil {
		return c.String(http.StatusInternalServerError, err.Error())
	}

	c.Response().Header().Set(echo.HeaderContentType, "text/csv; charset=utf-8")
	c.Response().Header().Set(echo.HeaderContentDisposition,
		fmt.Sprintf("attachment; filename=\"%s_comparison.csv\"", steamid))

	// Write CSV directly to the response writer
	if err := service.WriteCSV(c.Response(), rows); err != nil {
		return c.String(http.StatusInternalServerError, err.Error())
	}
	return nil
}

// POST /api/refresh/:steamid
// Triggers a refresh from Steam with throttling.
// - 200: { ok: true, gamesVisited, snapshots }
// - 429: { error: "throttled", retry_after_seconds: N } + Retry-After header
func (app *Application) Refresh(c echo.Context) error {
	steamid := c.Param("steamid")
	ctx := c.Request().Context()

	client, err := steamapi.New()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{"error": err.Error()})
	}

	// Throttle gate first (unchanged)
	if tw := config.ThrottleWindow(); tw > 0 {
		last, err := app.Repo.GetLastRefreshAt(ctx, steamid)
		if err == nil && !last.IsZero() {
			if remain := tw - time.Since(last); remain > 0 {
				sec := int((remain + time.Second - 1) / time.Second)
				c.Response().Header().Set("Retry-After", fmt.Sprintf("%d", sec))
				return c.JSON(http.StatusTooManyRequests, map[string]any{
					"error":               "throttled",
					"retry_after_seconds": sec,
				})
			}
		} else if err != nil && !errors.Is(err, db.ErrNoRows) {
			return c.JSON(http.StatusInternalServerError, map[string]any{"error": err.Error()})
		}
	}

	// Always concurrent with configured worker count
	workers := config.RefreshWorkers()
	stats, err := service.RefreshUserConcurrent(ctx, app.Repo, client, steamid, workers)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{"error": err.Error()})
	}

	// Update throttle timestamp
	if err := app.Repo.SetLastRefreshNow(ctx, steamid, time.Now().UTC()); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"ok":            true,
		"workers":       workers, // or config.RefreshWorkers()
		"owned":         stats.Owned,
		"queued":        stats.Queued,
		"checked":       stats.Checked,
		"updated":       stats.Updated,
		"skipped":       stats.Skipped,
		"skippedCached": stats.SkippedCached,
		"snapshots":     stats.Snapshots, // same as updated
	})
}

// GET /ui/results?steamid=...
func (app *Application) UIResults(c echo.Context) error {
	steamid := c.QueryParam("steamid")
	if steamid == "" {
		// Render the shell page if no steamid yet
		return views.Home().Render(c.Request().Context(), c.Response())
	}

	rows, err := service.BuildAllComparisonsForUser(c.Request().Context(), app.Repo, steamid)
	if err != nil {
		return c.String(http.StatusInternalServerError, err.Error())
	}
	return views.Results(steamid, rows).Render(c.Request().Context(), c.Response())
}

// POST /ui/refresh  (expects form field or hx-vals: steamid)
func (app *Application) UIRefresh(c echo.Context) error {
	steamid := c.FormValue("steamid")
	if steamid == "" {
		return c.String(http.StatusBadRequest, "missing steamid")
	}
	client, err := steamapi.New()
	if err != nil {
		return c.String(http.StatusInternalServerError, err.Error())
	}
	workers := config.RefreshWorkers()
	stats, err := service.RefreshUserConcurrent(c.Request().Context(), app.Repo, client, steamid, workers)
	if err != nil {
		return c.String(http.StatusInternalServerError, err.Error())
	}
	return views.RefreshStatus(steamid, workers, stats).Render(c.Request().Context(), c.Response())
}
