package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"onekey/internal/github"
	"onekey/internal/httpclient"
	"onekey/internal/i18n"
	"onekey/internal/models"
)

var httpClient = httpclient.Shared()

// fetchAppData builds game metadata + manifest info from the GitHub
// ManifestHub source (mirroring the original v1.5.1 client). The manifest
// binaries are pre-downloaded into Steam's depotcache here; the existing
// manifest.Handler pass in app.go then verifies/exists-checks them. No API key
// is required.
func fetchAppData(steamPath, appID string) (*models.SteamAppInfo, *models.SteamAppManifestInfo, error) {
	if !isDigits(appID) {
		return nil, nil, fmt.Errorf("%s", i18n.T("web.invalid_appid"))
	}

	res, err := github.BuildApp(appID)
	if err != nil {
		return nil, nil, err
	}

	// Download manifests into depotcache (best-effort; surviving ProcessManifests
	// re-checks existence and drops whatever failed to write).
	github.DownloadAll(steamPath, res.Download, nil)

	manifestInfo := res.ManifestInfo

	appInfo := &models.SteamAppInfo{
		AppID:                 appID,
		DLCCount:              len(manifestInfo.DLCs),
		DepotCount:            len(manifestInfo.MainApp),
		AccessToken:           "", // v1.5.1 emits no tokens; Setup() then skips addtoken()
		WorkshopDecryptionKey: "None",
	}

	// Name/header come from the public Steam store (no key needed).
	if name, img := fetchStoreMeta(appID); name != "" {
		appInfo.Name = name
		appInfo.HeaderImage = img
	}

	// When the input app_id is itself a DLC, config.json has no dlcs and all
	// branches carry the "main" manifests. If MainApp is empty but DLCs exist,
	// promote DLC manifests so they are processed + written to depotcache.
	if len(manifestInfo.MainApp) == 0 && len(manifestInfo.DLCs) > 0 {
		manifestInfo.MainApp = manifestInfo.DLCs
		manifestInfo.DLCs = nil
		appInfo.DepotCount = len(manifestInfo.MainApp)
	}

	return appInfo, manifestInfo, nil
}

// fetchStoreMeta reads the app's name and header image from the public Steam
// store appdetails endpoint.
func fetchStoreMeta(appID string) (name, header string) {
	data := fetchAppDetails(appID)
	if data == nil {
		return "", ""
	}
	var raw map[string]any
	if json.Unmarshal(data, &raw) != nil {
		return "", ""
	}
	d, ok := raw[appID].(map[string]any)
	if !ok {
		return "", ""
	}
	info, ok := d["data"].(map[string]any)
	if !ok {
		return "", ""
	}
	return getStringField(info, "name"), getStringField(info, "header_image")
}

func getStringField(m map[string]any, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
		return fmt.Sprintf("%v", v)
	}
	return ""
}

// intFieldStr extracts a JSON number field as an integer string, avoiding
// scientific notation from float64 (e.g. "4.11013e+06" → "4110130").
func intFieldStr(m map[string]any, key string) string {
	if v, ok := m[key]; ok {
		switch n := v.(type) {
		case float64:
			return fmt.Sprintf("%d", int64(n))
		case string:
			return n
		}
		return fmt.Sprintf("%v", v)
	}
	return ""
}

// searchStore resolves a game name to appids. The Steam store API is tried
// first with a short deadline (store.steampowered.com is in the DoH
// whitelist for clean DNS resolution). On failure we fall back to the免Key
// GitHub app list. SteamDB is deliberately skipped: it has Cloudflare
// anti-bot protection, so programmatic requests always get a challenge page.
func searchStore(term, lang string) (*models.StoreSearchResult, error) {
	// Store search over a short deadline so a blocked store domain can't stall
	// the whole search for the transport's long retry window.
	u := fmt.Sprintf("https://store.steampowered.com/api/storesearch/?term=%s&l=%s&cc=CN",
		url.QueryEscape(term), url.QueryEscape(lang))
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	if resp, err := httpClient.Do(req); err == nil {
		defer resp.Body.Close()
		data, _ := io.ReadAll(resp.Body)
		var result models.StoreSearchResult
		if json.Unmarshal(data, &result) == nil && len(result.Items) > 0 {
			return &result, nil
		}
	}

	// 免Key 兜底：官方 GetAppList 已下线，改用 GitHub app list 按名称查
	// AppID（经 CDN 镜像拉取并缓存），支持英文名/部分匹配。
	if items := github.SearchGames(term, lang, 20); len(items) > 0 {
		return &models.StoreSearchResult{Total: len(items), Items: items}, nil
	}
	return &models.StoreSearchResult{}, nil
}

// fetchParentApp queries Steam appdetails to check if appID is a DLC/music/etc.
// Returns (parentAppID, parentName) if it has a parent game, or ("", "") if not.
func fetchParentApp(appID string) (string, string) {
	data := fetchAppDetails(appID)
	if data == nil {
		return "", ""
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return "", ""
	}
	appData, ok := raw[appID].(map[string]any)
	if !ok {
		return "", ""
	}
	d, ok := appData["data"].(map[string]any)
	if !ok {
		return "", ""
	}
	// Any app with a "fullgame" field is a child (DLC, music, etc.)
	fg, ok := d["fullgame"].(map[string]any)
	if !ok {
		return "", ""
	}
	return getStringField(fg, "appid"), getStringField(fg, "name")
}

// fetchAppDetails fetches an app's details from the public Steam store.
func fetchAppDetails(appID string) []byte {
	u := fmt.Sprintf("https://store.steampowered.com/api/appdetails?appids=%s", appID)
	resp, err := httpClient.Get(u)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil || len(data) < 3 {
		return nil
	}
	return data
}

func testProxyConnectivity(proxyURL string) (bool, string) {
	c := httpclient.Shared()
	old := c.Proxy()
	if err := c.SetProxy(proxyURL); err != nil {
		return false, i18n.T("settings.proxy_invalid")
	}
	defer c.SetProxy(old)

	resp, err := c.Get("https://store.steampowered.com/api/storesearch/?term=test&cc=CN&l=schinese&count=1")
	if err != nil {
		return false, i18n.T("settings.proxy_fail", "error", err.Error())
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		return false, i18n.T("settings.proxy_fail", "error", fmt.Sprintf("HTTP %d", resp.StatusCode))
	}
	return true, i18n.T("settings.proxy_ok")
}
