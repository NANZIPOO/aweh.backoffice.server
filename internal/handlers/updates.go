package handlers

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
)

// VersionInfo represents the application version metadata
type VersionInfo struct {
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
	ServerVersion = VersionInfo{
		Version:     "1.0.0",
		BuildNumber: 1,
		GitHash:     "unknown",
		BuildDate:   "2026-03-07",
	}
)

func init() {
	// Try to load version from version.json
	data, err := os.ReadFile("version.json")
	if err == nil {
		var versionData VersionInfo
		if json.Unmarshal(data, &versionData) == nil {
			ServerVersion = versionData
		}
	}
}

// CheckForUpdates handles GET /api/v1/updates/check
func CheckForUpdates(w http.ResponseWriter, r *http.Request) {
	currentVersion := r.URL.Query().Get("current_version")
	platform := r.URL.Query().Get("platform")

	if currentVersion == "" {
		http.Error(w, "current_version parameter required", http.StatusBadRequest)
		return
	}

	// Compare versions
	updateAvailable := compareVersions(currentVersion, ServerVersion.Version)
	
	// Build download URL based on platform
	downloadURL := buildDownloadURL(platform, ServerVersion.Version)

	response := VersionCheckResponse{
		LatestVersion:   ServerVersion.Version,
		CurrentVersion:  currentVersion,
		UpdateAvailable: updateAvailable,
		Mandatory:       false, // Set to true for critical updates
		Changelog:       getChangelog(ServerVersion.Version),
		DownloadURL:     downloadURL,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// GetVersion handles GET /api/v1/version
func GetVersion(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ServerVersion)
}

// compareVersions returns true if server version is newer than client version
func compareVersions(client, server string) bool {
	// Simple semantic version comparison
	clientParts := strings.Split(strings.TrimPrefix(client, "v"), ".")
	serverParts := strings.Split(strings.TrimPrefix(server, "v"), ".")

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
// In production, this could read from a changelog.json file
func getChangelog(version string) string {
	// TODO: Implement changelog retrieval from file or database
	return "Bug fixes and performance improvements"
}
