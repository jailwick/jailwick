// Package update tells whether a newer jailwick is out, and updates it where Homebrew installed
// it. The app and the CLI share it, on every system: the check asks GitHub for the latest
// release (at most once a day, the answer cached); the update runs Homebrew, the only installer
// jailwick can drive, and anywhere else says where to download it.
package update

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jailwick/jailwick/internal/paths"
)

// Repo is where jailwick is released.
const Repo = "jailwick/jailwick"

// LatestURL answers with the latest published release (drafts and pre-releases are skipped).
var LatestURL = "https://api.github.com/repos/" + Repo + "/releases/latest"

// Daily is how old a cached answer may be before the next check asks again.
const Daily = 24 * time.Hour

// Release is a published version.
type Release struct {
	Version   string    `json:"version"` // without the v: 0.3.0
	URL       string    `json:"url"`     // its release page, with the downloads
	Notes     string    `json:"notes,omitempty"`
	Published time.Time `json:"published"`
}

// Status is what a check found.
type Status struct {
	Current   string    `json:"current"`
	Latest    *Release  `json:"latest,omitempty"` // nil until a check succeeded
	Available bool      `json:"available"`        // Latest is newer than Current
	Checked   time.Time `json:"checked,omitzero"`
	Error     string    `json:"error,omitempty"` // the last check failed (Latest is the one before)
}

// cache is the last check's answer, kept between runs.
type cache struct {
	Checked time.Time `json:"checked"`
	Latest  *Release  `json:"latest,omitempty"`
	Error   string    `json:"error,omitempty"`
	// Notified is the version the app last told about, so it tells once per version.
	Notified string `json:"notified,omitempty"`
}

// New is the checker for a running version, with its cache in jailwick's state folder.
func New(current string) *Checker {
	return &Checker{Current: current, File: filepath.Join(paths.StateDir(), "update.json")}
}

// Checker checks for a newer version than Current, caching the answer in File.
type Checker struct {
	Current string
	File    string
	Client  *http.Client // nil: a client with a short timeout
	Now     func() time.Time
}

func (c *Checker) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

// Check returns the status, asking GitHub only when the cached answer is older than maxAge (0:
// always). A failed check keeps the last known release and says why.
func (c *Checker) Check(ctx context.Context, maxAge time.Duration) Status {
	k := c.load()
	if maxAge == 0 || c.now().Sub(k.Checked) >= maxAge {
		rel, err := c.fetch(ctx)
		k.Checked, k.Error = c.now(), ""
		if err != nil {
			k.Error = err.Error()
		} else {
			k.Latest = rel
		}
		c.save(k)
	}
	return c.status(k)
}

// Cached returns the status from the last check, without asking anyone.
func (c *Checker) Cached() Status { return c.status(c.load()) }

func (c *Checker) status(k cache) Status {
	st := Status{Current: c.Current, Latest: k.Latest, Checked: k.Checked, Error: k.Error}
	st.Available = k.Latest != nil && Newer(k.Latest.Version, c.Current)
	return st
}

// ShouldNotify reports whether the available version wasn't told about yet, and records it as
// told.
func (c *Checker) ShouldNotify(st Status) bool {
	if !st.Available {
		return false
	}
	k := c.load()
	if k.Notified == st.Latest.Version {
		return false
	}
	k.Notified = st.Latest.Version
	c.save(k)
	return true
}

func (c *Checker) fetch(ctx context.Context) (*Release, error) {
	client := c.Client
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, LatestURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "jailwick/"+c.Current)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("couldn't reach GitHub: %w", err)
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return nil, errors.New("no release published yet")
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("GitHub answered %s", resp.Status)
	}
	var r struct {
		Tag       string    `json:"tag_name"`
		URL       string    `json:"html_url"`
		Body      string    `json:"body"`
		Published time.Time `json:"published_at"`
		Draft     bool      `json:"draft"`
		Pre       bool      `json:"prerelease"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&r); err != nil {
		return nil, fmt.Errorf("GitHub's answer: %w", err)
	}
	v := strings.TrimPrefix(r.Tag, "v")
	if r.Draft || r.Pre || parse(v) == nil {
		return nil, fmt.Errorf("the latest release %q isn't a version", r.Tag)
	}
	return &Release{Version: v, URL: r.URL, Notes: strings.TrimSpace(r.Body), Published: r.Published}, nil
}

func (c *Checker) load() cache {
	var k cache
	if data, err := os.ReadFile(c.File); err == nil {
		_ = json.Unmarshal(data, &k)
	}
	return k
}

func (c *Checker) save(k cache) {
	data, err := json.MarshalIndent(k, "", "  ")
	if err != nil {
		return
	}
	if os.MkdirAll(filepath.Dir(c.File), 0o755) == nil {
		tmp := c.File + ".tmp"
		if os.WriteFile(tmp, data, 0o644) == nil {
			_ = os.Rename(tmp, c.File)
		}
	}
}

// Newer reports whether version a is newer than b (major.minor.patch; anything after a - or +
// ranks below the plain version). An unreadable version is never newer.
func Newer(a, b string) bool {
	x, y := parse(a), parse(b)
	if x == nil || y == nil {
		return false
	}
	for i := range 3 {
		if x[i] != y[i] {
			return x[i] > y[i]
		}
	}
	return x[3] > y[3] // a release (1) is newer than its pre-release (0)
}

// parse reads major.minor.patch, plus 1 for a release or 0 for a pre-release; nil if unreadable.
func parse(v string) []int {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	release := 1
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		if v[i] == '-' {
			release = 0
		}
		v = v[:i]
	}
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return nil
	}
	out := make([]int, 0, 4)
	for _, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return nil
		}
		out = append(out, n)
	}
	return append(out, release)
}
