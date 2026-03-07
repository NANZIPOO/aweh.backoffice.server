package handler

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"
)

// ServerVersionInfo holds build-time version metadata
type ServerVersionInfo struct {
	Version     string `json:"version"`
	BuildNumber int    `json:"build_number"`
	GitHash     string `json:"git_hash"`
	BuildDate   string `json:"build_date"`
}

// VersionCheckResponse is the API response for version checks
type VersionCheckResponse struct {
	LatestVersion   string `json:"latest_version"`
	CurrentVersion  string `json:"current_version"`
	UpdateAvailable bool   `json:"update_available"`
	Mandatory       bool   `json:"mandatory"`
	Changelog       string `json:"changelog,omitempty"`
	DownloadURL     string `json:"download_url,omitempty"`
}

var (
	// ServerVersion is populated at build time or from version.json
	// These can be overridden via -ldflags at build time
	ServerVersion = ServerVersionInfo{
		Version:     "1.0.0",
		BuildNumber: 1,
		GitHash:     "unknown",
		BuildDate:   "2026-03-07",
	}
)

func init() {
	// Try to load version from version.json (for non-Docker deployments)
	data, err := os.ReadFile("version.json")
	if err == nil {
		var versionData ServerVersionInfo
		if json.Unmarshal(data, &versionData) == nil {
			ServerVersion = versionData
			log.Printf("Loaded version %s (build %d) from version.json",
				ServerVersion.Version, ServerVersion.BuildNumber)
		}
	}
}

// CheckVersionHandler handles GET /api/v1/updates/check
func CheckVersionHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	currentVersion := r.URL.Query().Get("current_version")
	platform := r.URL.Query().Get("platform")

	if currentVersion == "" {
		http.Error(w, "current_version parameter required", http.StatusBadRequest)
		return
	}

	log.Printf("Version check: current=%s, latest=%s, platform=%s, remote=%s",
		currentVersion, ServerVersion.Version, platform, r.RemoteAddr)

	// Compare versions
	updateAvailable := compareVersions(currentVersion, ServerVersion.Version)

	// Build download URL based on platform
	downloadURL := buildDownloadURL(platform, ServerVersion.Version)

	response := VersionCheckResponse{
		LatestVersion:   ServerVersion.Version,
		CurrentVersion:  currentVersion,
		UpdateAvailable: updateAvailable,
		Mandatory:       false, // Set to true for critical security updates
		Changelog:       getChangelog(ServerVersion.Version),
		DownloadURL:     downloadURL,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// GetVersionHandler handles GET /api/v1/version (returns server build info)
func GetVersionHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ServerVersion)
}

// compareVersions returns true if server version is newer than client version
func compareVersions(client, server string) bool {
	// Strip 'v' prefix if present
	client = strings.TrimPrefix(client, "v")
	server = strings.TrimPrefix(server, "v")

	// Simple semantic version comparison
	clientParts := strings.Split(client, ".")
	serverParts := strings.Split(server, ".")

	for i := 0; i < 3; i++ {
		var c, s int
		if i < len(clientParts) {
			c = atoi(clientParts[i])
		}
		if i < len(serverParts) {
			s = atoi(serverParts[i])
		}

		if s > c {
			return true // Server is newer
		} else if s < c {
			return false // Client is newer (shouldn't happen)
		}
	}

	return false // Versions are equal
}

func atoi(s string) int {
	var result int
	for _, c := range s {
		if c >= '0' && c <= '9' {
			result = result*10 + int(c-'0')
		}
	}
	return result
}

// buildDownloadURL constructs the download URL based on platform
func buildDownloadURL(platform, version string) string {
	// In production, this would point to your CDN or file server
	baseURL := os.Getenv("DOWNLOAD_BASE_URL")
	if baseURL == "" {
		baseURL = "https://updates.aweh.pos/releases"
	}

	switch platform {
	case "windows":
		return baseURL + "/aweh-pos-" + version + "-windows.msi"
	case "android":
		return baseURL + "/aweh-pos-" + version + ".apk"
	case "linux":
		return baseURL + "/aweh-pos-" + version + "-linux.AppImage"
	case "macos":
		return baseURL + "/aweh-pos-" + version + "-macos.dmg"
	default:
		return ""
	}
}

// getChangelog retrieves changelog for a version
// In production, this could read from a changelog.json file or database
func getChangelog(version string) string {
	// Try to load from changelog.json
	data, err := os.ReadFile("changelog.json")
	if err == nil {
		var changelogs map[string]string
		if json.Unmarshal(data, &changelogs) == nil {
			if text, ok := changelogs[version]; ok {
				return text
			}
		}
	}

	// Default changelog
	return "Bug fixes and performance improvements"
}
