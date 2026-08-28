// Package github provides the Steam manifest data source used by Onekey,
// mirroring the original v1.5.1 (ikunshare/Onekey) implementation: per-app
// branches in public ManifestHub repositories are resolved via the GitHub
// API and their .manifest / Key.vdf files are fetched through raw-content
// CDN mirrors. Unlike the ok.wwwweb.top backend, this needs no API key.
package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"onekey/internal/httpclient"
	"onekey/internal/manifest"
	"onekey/internal/models"
)

var httpClient = httpclient.Shared()

// reqTimeout bounds each individual mirror attempt so a mirror that accepts
// the connection but never responds (common on restricted networks) fails
// fast and lets fetchFile try the next mirror instead of stalling the unlock.
const reqTimeout = 20 * time.Second

// repos is the ordered repository list to scan for a branch. The branch with
// the most recent commit wins. Missing branches in any repo are skipped, so
// adding current 2026 sources (steamtools-games/ManifestHub3, actively
// maintained) only widens coverage and cannot break existing lookups.
var repos = []string{
	"SteamAutoCracks/ManifestHub",
	"ikun0014/ManifestHub",
	"Auiowu/ManifestAutoUpdate",
	"tymolu233/ManifestAutoUpdate-fix",
	"steamtools-games/ManifestHub3",
}

// cdnTemplates are raw-content mirrors (CN-friendly first; the blocked direct
// raw.githubusercontent.com is tried last so a GW-blocked connection never
// stalls manifest download). Placeholders {repo}, {sha}, {path} are substituted
// per template. fetchFile walks them in order and falls through on any failure,
// so an unreachable mirror is a no-op, never a blocker.
var cdnTemplates = []string{
	"https://cdn.jsdmirror.com/gh/{repo}@{sha}/{path}",
	"https://raw.gitmirror.com/{repo}/{sha}/{path}",
	"https://raw.dgithub.xyz/{repo}/{sha}/{path}",
	"https://gh.akass.cn/{repo}/{sha}/{path}",
	"https://ghfast.top/{repo}/{sha}/{path}",
	"https://ghproxy.net/{repo}/{sha}/{path}",
	"https://raw.githubusercontent.com/{repo}/{sha}/{path}",
}

const apiBase = "https://api.github.com"

// resolved identifies a branch of a repository that carries games data.
type resolved struct {
	repo    string
	sha     string // branch name (usable as the ref in raw URLs)
	treeURL string
	date    time.Time
}

// depotFile describes one .manifest file to download into depotcache.
type depotFile struct {
	DepotID    string
	ManifestID string
	Name       string // path within the branch
	Repo       string
	Sha        string
}

// BuildResult carries the manifests (for Lua generation) and the files that
// must be downloaded into Steam's depotcache.
type BuildResult struct {
	ManifestInfo *models.SteamAppManifestInfo
	Download     []depotFile
}

// getBytesTimeout GETs a URL with a per-attempt timeout.
func getBytesTimeout(url string, timeout time.Duration) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	resp, err := httpClient.GetCtx(ctx, url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		io.Copy(io.Discard, resp.Body) // ponytail: drain so the conn can be pooled
		return nil, fmt.Errorf("github: http %d for %s", resp.StatusCode, url)
	}
	return io.ReadAll(resp.Body)
}

// getBytes GETs a URL and returns the body, erroring on non-200 or timeout.
func getBytes(url string) ([]byte, error) {
	return getBytesTimeout(url, reqTimeout)
}

// apiMirrorPrefixes are generic prefix proxies that forward any github URL,
// including api.github.com which raw-only mirrors can't relay. Domestic-first:
// in CN api.github.com is blocked, so direct access sits last (still tried, but
// only after every mirror fails). This lets the whole GitHub data path work
// without Steam++ (which conflicts with the unlock kernel). ponytail: prefix
// proxies are a best-effort ladder; reliability varies by network.
var apiMirrorPrefixes = []string{
	"https://ghfast.top/",
	"https://ghproxy.net/",
	"https://gh-proxy.com/",
	"https://ghproxy.cc/",
	"https://github.moeyy.xyz/",
	"https://ghps.cc/",
	"https://gh.llkk.cc/",
	"", // direct, last: usually blocked/slow in CN
}

// apiMirrorTimeout bounds each mirror attempt so a blocked mirror (or blocked
// direct API) fails fast instead of stalling the unlock for tens of seconds.
// Mirrors either answer quickly (200/404) or are blocked; 3s flushes the
// blocked ones without holding tens of seconds per prefix.
const apiMirrorTimeout = 3 * time.Second

// race fetches each attempt in parallel and returns the first successful body.
// Overall wall time ≈ the fastest mirror, instead of the old serial scan where
// a slow/live-blocked mirror near the top of the list stalls every download for
// its full timeout (7 × 20s worst case). This is the Go-equivalent of Steam++'s
// reverse-proxy node racing; no separate proxy, hosts, or admin rights needed.
func race(get func(attempt int) ([]byte, error), n int) ([]byte, error) {
	type result struct {
		b   []byte
		err error
	}
	ch := make(chan result, n)
	for i := 0; i < n; i++ {
		go func(i int) {
			b, err := get(i)
			ch <- result{b, err}
		}(i)
	}

	var lastErr error
	for i := 0; i < n; i++ {
		r := <-ch
		if r.err == nil {
			return r.b, nil
		}
		lastErr = r.err
	}
	return nil, lastErr
}

// getBytesViaMirrors races the prefix proxies in parallel; the first proxy that
// returns OK wins, so a live-blocked mirror never stalls the whole unlock.
func getBytesViaMirrors(url string) ([]byte, error) {
	return race(func(i int) ([]byte, error) {
		return getBytesTimeout(apiMirrorPrefixes[i]+url, apiMirrorTimeout)
	}, len(apiMirrorPrefixes))
}

// queryBranch asks the GitHub API whether repo has the branch named branch.
func queryBranch(repo, branch string) (*resolved, bool, error) {
	body, err := getBytesViaMirrors(fmt.Sprintf("%s/repos/%s/branches/%s", apiBase, repo, branch))
	if err != nil || len(body) < 3 || bytesContains(body, "Not Found") {
		return nil, false, nil // absent repo/branch is a normal "not found", not fatal
	}
	var raw struct {
		Commit struct {
			Sha    string `json:"sha"`
			Commit struct {
				Committer struct {
					Date string `json:"date"`
				} `json:"committer"`
				Tree struct {
					URL string `json:"url"`
				} `json:"tree"`
			} `json:"commit"`
		} `json:"commit"`
	}
	if json.Unmarshal(body, &raw) != nil || raw.Commit.Sha == "" {
		return nil, false, nil
	}
	date, _ := time.Parse(time.RFC3339, raw.Commit.Commit.Committer.Date)
	return &resolved{
		repo:    repo,
		sha:     branch,
		treeURL: raw.Commit.Commit.Tree.URL + "?recursive=1",
		date:    date,
	}, true, nil
}

func bytesContains(b []byte, s string) bool {
	return strings.Contains(strings.ToLower(string(b)), strings.ToLower(s))
}

// resolveBranch scans each repo for the branch branchID and returns the first
// repo that has it. The ManifestHub repos mirror the same upstream data, so
// picking the first hit is fast (mirrors can be slow) without meaningful loss.
// ponytail: drops the "most recently updated repo wins" tie-breaker that
// scanned every repo; acceptable since all sources are near-identical dumps.
func resolveBranch(branchID string) (*resolved, error) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Scan all repos in parallel so the missing-branch latency of one repo
	// never stacks serially onto the next. A branch exists in only one source,
	// but each queryBranch() already waits its own mirror timeout on a miss, so
	// serializing 5 repos was the #1 cause of "正在获取游戏…" hanging for tens
	// of seconds. Race them; the first hit wins.
	results := make(chan *resolved, len(repos))
	var wg sync.WaitGroup
	for _, r := range repos {
		wg.Add(1)
		go func(repo string) {
			defer wg.Done()
			rs, ok, _ := queryBranch(repo, branchID)
			if ok && rs != nil {
				select {
				case results <- rs:
				case <-ctx.Done():
				}
			}
		}(r)
	}
	go func() {
		wg.Wait()
		close(results)
	}()

	for rs := range results {
		cancel()
		return rs, nil
	}
	return nil, fmt.Errorf("github: no data branch found for app %s", branchID)
}

// resolveBranchPinned prefers preferRepo for the branch, falling back to a
// full scan. DLC branches usually live in the same repo as their parent game,
// so this saves API round-trips.
func resolveBranchPinned(branchID, preferRepo string) (*resolved, error) {
	if preferRepo != "" {
		if rs, ok, _ := queryBranch(preferRepo, branchID); ok && rs != nil {
			return rs, nil
		}
	}
	return resolveBranch(branchID)
}

// listFiles returns the recursive file paths of a branch's git tree.
func listFiles(rs *resolved) ([]string, error) {
	body, err := getBytesViaMirrors(rs.treeURL)
	if err != nil {
		return nil, err
	}
	var raw struct {
		Tree []struct {
			Path string `json:"path"`
		} `json:"tree"`
	}
	if json.Unmarshal(body, &raw) != nil {
		return nil, fmt.Errorf("github: bad tree response for %s", rs.repo)
	}
	out := make([]string, 0, len(raw.Tree))
	for _, t := range raw.Tree {
		out = append(out, t.Path)
	}
	return out, nil
}

// fetchFile downloads a file from a branch, racing all CDN mirrors in parallel
// so a slow top-of-list mirror cannot stall the manifest download.
func fetchFile(rs *resolved, filePath string) ([]byte, error) {
	return race(func(i int) ([]byte, error) {
		t := cdnTemplates[i]
		u := strings.NewReplacer("{repo}", rs.repo, "{sha}", rs.sha, "{path}", filePath).Replace(t)
		return getBytes(u)
	}, len(cdnTemplates))
}

// parseManifestName splits a "{depot}_{manifest}.manifest" filename.
func parseManifestName(name string) (depot, manifestID string, ok bool) {
	base := strings.TrimSuffix(name, ".manifest")
	parts := strings.Split(base, "_")
	if len(parts) != 2 {
		return "", "", false
	}
	if !allDigits(parts[0]) || !allDigits(parts[1]) {
		return "", "", false
	}
	return parts[0], parts[1], true
}

func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// parseKeyVDF extracts depot_id -> DecryptionKey from a Key.vdf block. The
// generated format is a simple VDF:
//
//	"depots" { "1144201" { "DecryptionKey" "b923..." } }
var keyVDFRe = regexp.MustCompile(`"(\d+)"\s*\{\s*"DecryptionKey"\s*"([0-9a-fA-F]+)"`)

func parseKeyVDF(content []byte) map[string]string {
	keys := make(map[string]string)
	for _, m := range keyVDFRe.FindAllSubmatch(content, -1) {
		keys[string(m[1])] = string(m[2])
	}
	return keys
}

type branchData struct {
	manifests []models.ManifestInfo
	files     []depotFile
}

// parseBranch turns a branch's file listing into manifest metadata + download
// targets. Only .manifest files and Key.vdf are consumed.
func parseBranch(appID string, rs *resolved, paths []string) branchData {
	keys := map[string]string{}
	var keyPath string
	var data branchData

	for _, p := range paths {
		lower := strings.ToLower(path.Base(p))
		switch {
		case lower == "key.vdf":
			if keyPath == "" {
				keyPath = p
			}
		case strings.HasSuffix(lower, ".manifest"):
			if depotID, mid, ok := parseManifestName(lower); ok {
				data.files = append(data.files, depotFile{depotID, mid, p, rs.repo, rs.sha})
				data.manifests = append(data.manifests,
					models.ManifestInfo{AppID: appID, DepotID: depotID, ManifestID: mid})
			}
		}
	}
	if keyPath != "" {
		if b, err := fetchFile(rs, keyPath); err == nil {
			keys = parseKeyVDF(b)
		}
	}
	for i := range data.manifests {
		data.manifests[i].DepotKey = keys[data.manifests[i].DepotID]
	}
	return data
}

// --- 免Key game-name → AppID search ---
//
// Steam's public GetAppList/v2 is shut down ("Method not found") and the
// replacement IStoreService/GetAppList requires an API key. Name→AppID lookups
// therefore use a community-maintained app list (jsnli/steamappidlist) fetched
// through the same CDN mirror ladder as manifests and cached in memory after
// the first hit, so searching needs no key and reuses the proven CN-friendly
// mirror infra. DLC entries are skipped — users searching a game name want the
// base app.

const appListRepo = "jsnli/steamappidlist"
const appListFile = "data/games_appid.json"
const appListBranch = "master"

var (
	appListMu sync.Mutex
	appList   []models.StoreSearchItem
)

// SearchGames returns up to limit titles whose name contains term (caseinsensitive,
// prefix matches ranked first). Returns nil if the app list can't be fetched.
func SearchGames(term, lang string, limit int) []models.StoreSearchItem {
	items := loadAppList()
	if len(items) == 0 {
		return nil
	}
	needle := strings.ToLower(strings.TrimSpace(term))
	if needle == "" {
		return nil
	}
	var pref, sub []models.StoreSearchItem
	for _, it := range items {
		hay := strings.ToLower(it.Name)
		switch {
		case strings.HasPrefix(hay, needle):
			pref = append(pref, it)
		case strings.Contains(hay, needle):
			sub = append(sub, it)
		}
	}
	ranked := append(pref, sub...)
	if len(ranked) > limit {
		ranked = ranked[:limit]
	}
	return ranked
}

// loadAppList fetches the免Key app list once through the CDN mirror ladder and
// caches it in memory. lang is intentionally unused — the mirrored dataset only
// carries English titles, but store search handles localized names as its
// primary path, so GitHub hits are a same-script fallback.
func loadAppList() []models.StoreSearchItem {
	appListMu.Lock()
	defer appListMu.Unlock()
	if appList != nil {
		return appList
	}
	b, err := fetchFile(&resolved{repo: appListRepo, sha: appListBranch}, appListFile)
	if err != nil {
		return nil
	}
	var arr []struct {
		AppID int    `json:"appid"`
		Name  string `json:"name"`
	}
	if err := json.Unmarshal(b, &arr); err != nil {
		return nil
	}
	out := make([]models.StoreSearchItem, 0, len(arr))
	for _, a := range arr {
		if a.AppID == 0 || a.Name == "" {
			continue
		}
		out = append(out, models.StoreSearchItem{
			Type: "app",
			Name: a.Name,
			ID:   a.AppID,
			// Platforms unknown from the list; present all so the card shows
			// the platform chips like a normal store result.
			Platforms: &models.StoreSearchPlatforms{Windows: true, Mac: true, Linux: true},
			TinyImage: fmt.Sprintf("https://cdn.cloudflare.steamstatic.com/steam/apps/%d/capsule_231x87.jpg", a.AppID),
		})
	}
	appList = out
	return appList
}

func hasPath(paths []string, name string) bool {
	for _, p := range paths {
		if strings.EqualFold(path.Base(p), name) {
			return true
		}
	}
	return false
}

func dedupeDownloads(files []depotFile) []depotFile {
	seen := make(map[string]bool, len(files))
	out := files[:0]
	for _, f := range files {
		k := f.DepotID + "_" + f.ManifestID
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, f)
	}
	return out
}

// BuildApp resolves the data branch for appID (and its DLC branches listed in
// config.json), builds the manifest metadata, and returns the files to
// download. No files are written here.
func BuildApp(appID string) (*BuildResult, error) {
	rs, err := resolveBranch(appID)
	if err != nil {
		return nil, err
	}
	paths, err := listFiles(rs)
	if err != nil {
		return nil, err
	}

	bd := parseBranch(appID, rs, paths)
	info := &models.SteamAppManifestInfo{MainApp: bd.manifests}
	res := &BuildResult{ManifestInfo: info, Download: bd.files}

	// config.json lists the DLC app ids; each DLC has its own branch/depots.
	if hasPath(paths, "config.json") {
		if cfg, err := fetchFile(rs, "config.json"); err == nil {
			var c struct {
				DLCs []int `json:"dlcs"`
			}
			if json.Unmarshal(cfg, &c) == nil {
				for _, id := range c.DLCs {
					dlcID := strconv.Itoa(id)
					drs, derr := resolveBranchPinned(dlcID, rs.repo)
					if derr != nil {
						continue
					}
					dp, derr := listFiles(drs)
					if derr != nil {
						continue
					}
					dd := parseBranch(dlcID, drs, dp)
					info.DLCs = append(info.DLCs, dd.manifests...)
					res.Download = append(res.Download, dd.files...)
				}
			}
		}
	}

	res.Download = dedupeDownloads(res.Download)
	return res, nil
}

// DownloadAll writes each manifest binary into Steam's depotcache as
// "{depot}_{manifest}.manifest", skipping files that already exist.
// Files are downloaded concurrently (each is an independent fetch/write into a
// distinct file) so N manifests with slow mirrors cost ~1 slow download instead
// of N slow downloads summed serially. onProgress, when non-nil, is called per
// file as (depotID, success).
func DownloadAll(steamPath string, files []depotFile, onProgress func(depotID string, ok bool)) {
	if len(files) == 0 {
		return
	}
	cache := filepath.Join(steamPath, "depotcache")
	os.MkdirAll(cache, 0755)

	const concurrency = 5 // ponytail: cap to avoid fanning out onto flaky public mirrors
	results := make([]bool, len(files))
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	for i, f := range files {
		wg.Add(1)
		go func(i int, f depotFile) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			target := filepath.Join(cache, f.DepotID+"_"+f.ManifestID+".manifest")
			ok := false
			if _, err := os.Stat(target); err == nil {
				ok = true
			} else if b, err := fetchFile(&resolved{repo: f.Repo, sha: f.Sha}, f.Name); err == nil {
				if os.WriteFile(target, manifest.ExtractManifestPayload(b), 0644) == nil {
					ok = true
				}
			}
			results[i] = ok
		}(i, f)
	}
	wg.Wait()
	for i, ok := range results {
		if onProgress != nil {
			onProgress(files[i].DepotID, ok)
		}
	}
}
