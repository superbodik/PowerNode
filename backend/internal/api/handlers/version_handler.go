package handlers

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var versionPattern = regexp.MustCompile(`^\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?$`)
var semverPattern = regexp.MustCompile(`^(\d+)\.(\d+)\.(\d+)(?:-([0-9A-Za-z.-]+))?$`)

type VersionHandler struct {
	Version   string
	Commit    string
	BuildDate string
	SourceDir string
	RepoSlug  string
}

func (h *VersionHandler) Get(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"version":    h.Version,
		"commit":     h.Commit,
		"build_date": h.BuildDate,
		"source_dir": h.SourceDir,
		"repo_slug":  h.RepoSlug,
	})
}

func (h *VersionHandler) CheckUpdate(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	latest, err := latestReleasedVersion(ctx, h.RepoSlug)
	if err != nil {
		http.Error(w, "failed to reach GitHub: "+err.Error(), http.StatusBadGateway)
		return
	}

	updateAvailable := h.Version != "" && h.Version != "0.0.0-dev" && latest != "" && isNewerVersion(latest, h.Version)

	writeJSON(w, http.StatusOK, map[string]any{
		"current_version":  h.Version,
		"latest_version":   latest,
		"update_available": updateAvailable,
	})
}

type semver struct {
	major, minor, patch int
	pre                 string
}

func parseSemver(v string) (semver, bool) {
	m := semverPattern.FindStringSubmatch(v)
	if m == nil {
		return semver{}, false
	}
	major, _ := strconv.Atoi(m[1])
	minor, _ := strconv.Atoi(m[2])
	patch, _ := strconv.Atoi(m[3])
	return semver{major: major, minor: minor, patch: patch, pre: m[4]}, true
}

// isNewerVersion reports whether a has higher SemVer precedence than b.
// A missing pre-release beats any pre-release (1.0.0 > 1.0.0-rc), and falls
// back to a plain inequality check if either string isn't parseable SemVer.
func isNewerVersion(a, b string) bool {
	pa, oka := parseSemver(a)
	pb, okb := parseSemver(b)
	if !oka || !okb {
		return a != b
	}
	if pa.major != pb.major {
		return pa.major > pb.major
	}
	if pa.minor != pb.minor {
		return pa.minor > pb.minor
	}
	if pa.patch != pb.patch {
		return pa.patch > pb.patch
	}
	return comparePreRelease(pa.pre, pb.pre) > 0
}

func comparePreRelease(a, b string) int {
	if a == b {
		return 0
	}
	if a == "" {
		return 1
	}
	if b == "" {
		return -1
	}
	aParts := strings.Split(a, ".")
	bParts := strings.Split(b, ".")
	for i := 0; i < len(aParts) && i < len(bParts); i++ {
		ap, bp := aParts[i], bParts[i]
		if ap == bp {
			continue
		}
		an, aErr := strconv.Atoi(ap)
		bn, bErr := strconv.Atoi(bp)
		if aErr == nil && bErr == nil {
			if an != bn {
				if an > bn {
					return 1
				}
				return -1
			}
			continue
		}
		if aErr == nil {
			return -1
		}
		if bErr == nil {
			return 1
		}
		if ap < bp {
			return -1
		}
		return 1
	}
	if len(aParts) != len(bParts) {
		if len(aParts) < len(bParts) {
			return -1
		}
		return 1
	}
	return 0
}

func latestReleasedVersion(ctx context.Context, repoSlug string) (string, error) {
	url := "https://raw.githubusercontent.com/" + repoSlug + "/main/VERSION"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("GitHub returned %d fetching VERSION", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	version := strings.TrimSpace(string(body))
	if !versionPattern.MatchString(version) {
		return "", fmt.Errorf("unexpected VERSION file contents from GitHub")
	}
	return version, nil
}
