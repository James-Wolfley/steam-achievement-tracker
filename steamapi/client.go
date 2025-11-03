package steamapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"
)

type Client struct {
	key    string
	client *http.Client
}

// New reads STEAM_API_KEY and returns a client with sensible timeouts.
func New() (*Client, error) {
	key := os.Getenv("STEAM_API_KEY")
	if key == "" {
		return nil, errors.New("STEAM_API_KEY not set")
	}
	return &Client{
		key: key,
		client: &http.Client{
			Timeout: 20 * time.Second,
			Transport: &http.Transport{
				Proxy:                 http.ProxyFromEnvironment,
				DialContext:           (&net.Dialer{Timeout: 10 * time.Second}).DialContext,
				TLSHandshakeTimeout:   10 * time.Second,
				ExpectContinueTimeout: 1 * time.Second,
				IdleConnTimeout:       30 * time.Second,
				MaxIdleConns:          100,
				MaxConnsPerHost:       10,
			},
		},
	}, nil
}

// ------------ API shapes ------------

type OwnedGamesResp struct {
	Response struct {
		GameCount int         `json:"game_count"`
		Games     []OwnedGame `json:"games"`
	} `json:"response"`
}

type OwnedGame struct {
	AppID                    int64  `json:"appid"`
	Name                     string `json:"name"`
	HasCommunityVisibleStats bool   `json:"has_community_visible_stats"`
	PlaytimeForever          int    `json:"playtime_forever"`
}

type SchemaForGameResp struct {
	Game struct {
		GameName           string `json:"gameName"`
		GameVersion        string `json:"gameVersion"`
		AvailableGameStats struct {
			Achievements []struct {
				Name        string `json:"name"`        // apiname
				DisplayName string `json:"displayName"` // human name
				Description string `json:"description"`
			} `json:"achievements"`
		} `json:"availableGameStats"`
	} `json:"game"`
}

type PlayerAchievementsResp struct {
	Playerstats struct {
		SteamID      string `json:"steamID"`
		GameName     string `json:"gameName"`
		Achievements []struct {
			APIName  string `json:"apiname"`
			Achieved int    `json:"achieved"` // 0/1
			UnlockTs int64  `json:"unlocktime"`
		} `json:"achievements"`
		Success bool `json:"success"`
	} `json:"playerstats"`
}

// ------------ Calls ------------

// GetOwnedGames returns the user's owned games, including names.
func (c *Client) GetOwnedGames(ctx context.Context, steamid string) ([]OwnedGame, error) {
	u := "https://api.steampowered.com/IPlayerService/GetOwnedGames/v1/"
	q := url.Values{}
	q.Set("key", c.key)
	q.Set("steamid", steamid)
	q.Set("include_appinfo", "1")
	q.Set("include_played_free_games", "1")
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u+"?"+q.Encode(), nil)

	var out OwnedGamesResp
	if err := c.doJSON(req, &out); err != nil {
		return nil, err
	}
	return out.Response.Games, nil
}

// GetSchemaForGame lists achievement defs for an app. Some games have no achievements.
func (c *Client) GetSchemaForGame(ctx context.Context, appid int64) (defs []SchemaDef, gameName string, err error) {
	u := "https://api.steampowered.com/ISteamUserStats/GetSchemaForGame/v2/"
	q := url.Values{}
	q.Set("key", c.key)
	q.Set("appid", strconv.FormatInt(appid, 10))
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u+"?"+q.Encode(), nil)

	var raw SchemaForGameResp
	if err := c.doJSON(req, &raw); err != nil {
		return nil, "", err
	}
	gameName = raw.Game.GameName
	for _, a := range raw.Game.AvailableGameStats.Achievements {
		defs = append(defs, SchemaDef{
			APIName: a.Name,
			Name:    emptyFallback(a.DisplayName, a.Name),
			Descr:   a.Description,
		})
	}
	return defs, gameName, nil
}

// GetPlayerAchievements returns achievement states for a user/app.
// If the game has no achievements or stats are hidden, Steam may return success=false.
func (c *Client) GetPlayerAchievements(ctx context.Context, steamid string, appid int64) ([]PlayerAch, error) {
	u := "https://api.steampowered.com/ISteamUserStats/GetPlayerAchievements/v1/"
	q := url.Values{}
	q.Set("key", c.key)
	q.Set("steamid", steamid)
	q.Set("appid", strconv.FormatInt(appid, 10))
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u+"?"+q.Encode(), nil)

	var raw PlayerAchievementsResp
	if err := c.doJSON(req, &raw); err != nil {
		return nil, err
	}
	ach := make([]PlayerAch, 0, len(raw.Playerstats.Achievements))
	for _, a := range raw.Playerstats.Achievements {
		ach = append(ach, PlayerAch{
			APIName:  a.APIName,
			Achieved: a.Achieved == 1,
			UnlockTs: a.UnlockTs,
		})
	}
	return ach, nil
}

// ------------ Types used by service ------------

type SchemaDef struct {
	APIName string
	Name    string
	Descr   string
}

type PlayerAch struct {
	APIName  string
	Achieved bool
	UnlockTs int64
}

// ------------ internals ------------

func (c *Client) doJSON(req *http.Request, v any) error {
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("steam http %d", resp.StatusCode)
	}
	dec := json.NewDecoder(resp.Body)
	return dec.Decode(v)
}

func emptyFallback(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}
