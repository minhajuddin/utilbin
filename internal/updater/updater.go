// Package updater self-replaces the running binary by pulling the latest
// release tarball from GitHub. The check runs in a background goroutine on
// every invocation; a timestamp file gates how often the network is touched.
package updater

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	repo                  = "minhajuddin/utilbin"
	defaultCheckInterval  = 24 * time.Hour
	envAutoUpdate         = "UTILBIN_AUTO_UPDATE"
	envCheckInterval      = "UTILBIN_UPDATE_INTERVAL"
	envCheckURL           = "UTILBIN_RELEASES_URL" // override for testing
	httpTimeout           = 30 * time.Second
	lastCheckFileBasename = "last-check"
)

// MaybeBackgroundCheck kicks off a non-blocking update check. The current
// version is the binary's own; if a newer release exists it's downloaded and
// the running binary is replaced atomically. Pure best-effort — any error is
// silently swallowed so it never disturbs the user-facing command.
func MaybeBackgroundCheck(currentVersion string) {
	if os.Getenv(envAutoUpdate) == "0" {
		return
	}
	if currentVersion == "dev" || currentVersion == "" {
		// Local dev builds shouldn't ever try to overwrite themselves with a
		// release artifact — silently skip.
		return
	}

	interval := defaultCheckInterval
	if v := os.Getenv(envCheckInterval); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			interval = d
		}
	}

	if !shouldCheck(interval) {
		return
	}

	go func() {
		defer func() { _ = recover() }()
		_ = touchLastCheck() // mark attempt before network so a hang doesn't busy-loop
		ctx, cancel := context.WithTimeout(context.Background(), httpTimeout)
		defer cancel()
		_, _ = checkAndApply(ctx, currentVersion, false)
	}()
}

// CheckAndApply runs the update synchronously, returning a human-readable
// status. Used by the explicit `utilbin update` command.
func CheckAndApply(ctx context.Context, currentVersion string, force, checkOnly bool) (string, error) {
	rel, err := fetchLatest(ctx)
	if err != nil {
		return "", err
	}
	if !force && !isNewer(rel.TagName, currentVersion) {
		return fmt.Sprintf("Already up to date (%s).", currentVersion), nil
	}
	if checkOnly {
		return fmt.Sprintf("New release available: %s (current %s).", rel.TagName, currentVersion), nil
	}
	if err := apply(ctx, rel); err != nil {
		return "", err
	}
	return fmt.Sprintf("Updated to %s.", rel.TagName), nil
}

func checkAndApply(ctx context.Context, currentVersion string, force bool) (string, error) {
	return CheckAndApply(ctx, currentVersion, force, false)
}

// --- internals ---

type ghRelease struct {
	TagName string    `json:"tag_name"`
	Assets  []ghAsset `json:"assets"`
}

type ghAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

func releasesURL() string {
	if v := os.Getenv(envCheckURL); v != "" {
		return v
	}
	return "https://api.github.com/repos/" + repo + "/releases/latest"
}

func fetchLatest(ctx context.Context) (*ghRelease, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", releasesURL(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GitHub API returned %d", resp.StatusCode)
	}
	var rel ghRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, err
	}
	if rel.TagName == "" {
		return nil, errors.New("empty tag in release response")
	}
	return &rel, nil
}

// isNewer compares two semver-ish tags ("v1.2.3" vs "v1.2.4"). Falls back to
// string compare if parsing fails — good enough since goreleaser produces
// well-formed tags.
func isNewer(latest, current string) bool {
	return strings.TrimPrefix(latest, "v") != strings.TrimPrefix(current, "v") &&
		strings.TrimPrefix(latest, "v") > strings.TrimPrefix(current, "v")
}

func apply(ctx context.Context, rel *ghRelease) error {
	tarballName := fmt.Sprintf("utilbin_%s_%s.tar.gz", runtime.GOOS, runtime.GOARCH)
	checksumName := "checksums.txt"

	var tarballURL, checksumURL string
	for _, a := range rel.Assets {
		switch a.Name {
		case tarballName:
			tarballURL = a.BrowserDownloadURL
		case checksumName:
			checksumURL = a.BrowserDownloadURL
		}
	}
	if tarballURL == "" {
		return fmt.Errorf("no asset matching %s in release %s", tarballName, rel.TagName)
	}

	wantSum := ""
	if checksumURL != "" {
		body, err := httpGet(ctx, checksumURL)
		if err != nil {
			return fmt.Errorf("fetching checksums: %w", err)
		}
		wantSum = lookupChecksum(string(body), tarballName)
	}

	tarball, err := httpGet(ctx, tarballURL)
	if err != nil {
		return fmt.Errorf("downloading tarball: %w", err)
	}
	if wantSum != "" {
		gotSum := sha256hex(tarball)
		if gotSum != wantSum {
			return fmt.Errorf("checksum mismatch: got %s want %s", gotSum, wantSum)
		}
	}

	binBytes, err := extractBinary(tarball, "utilbin")
	if err != nil {
		return fmt.Errorf("extracting binary: %w", err)
	}

	return swapBinary(binBytes)
}

func httpGet(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d for %s", resp.StatusCode, url)
	}
	return io.ReadAll(resp.Body)
}

func sha256hex(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// lookupChecksum scans a `sha256  filename` style file (goreleaser format)
// and returns the hash matching name, or "" if not present.
func lookupChecksum(body, name string) string {
	for line := range strings.SplitSeq(body, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[1] == name {
			return fields[0]
		}
	}
	return ""
}

func extractBinary(tarball []byte, binName string) ([]byte, error) {
	gz, err := gzip.NewReader(strings.NewReader(string(tarball)))
	if err != nil {
		return nil, err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if filepath.Base(hdr.Name) == binName {
			return io.ReadAll(tr)
		}
	}
	return nil, fmt.Errorf("%s not found in tarball", binName)
}

// swapBinary atomically replaces the currently-running binary with new bytes.
// On Linux/macOS, os.Rename works even on the executing file because the
// kernel keeps the old inode alive until the process exits.
func swapBinary(newBytes []byte) error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	self, err = filepath.EvalSymlinks(self)
	if err != nil {
		return err
	}
	dir := filepath.Dir(self)
	tmp, err := os.CreateTemp(dir, ".utilbin-update-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath) // no-op once Rename succeeds
	if _, err := tmp.Write(newBytes); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o755); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, self)
}

// --- timestamp gating ---

func cacheDir() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "utilbin")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

func lastCheckPath() (string, error) {
	d, err := cacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, lastCheckFileBasename), nil
}

func shouldCheck(interval time.Duration) bool {
	p, err := lastCheckPath()
	if err != nil {
		return false
	}
	info, err := os.Stat(p)
	if err != nil {
		return true // never checked
	}
	return time.Since(info.ModTime()) > interval
}

func touchLastCheck() error {
	p, err := lastCheckPath()
	if err != nil {
		return err
	}
	now := time.Now()
	if f, err := os.Create(p); err == nil {
		f.Close()
	}
	return os.Chtimes(p, now, now)
}
