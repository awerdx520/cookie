package native

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// ExportFile holds the on-disk format of exported cookies.
type ExportFile struct {
	Timestamp int64                      `json:"timestamp"`
	Cookies   map[string]json.RawMessage `json:"cookies"`
}

// ExportDir returns ~/.cookie, creating it if necessary.
func ExportDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".cookie")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	return dir, nil
}

// ExportFilePath returns ~/.cookie/export.json.
func ExportFilePath() (string, error) {
	dir, err := ExportDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "export.json"), nil
}

// WriteExport writes (or merges) cookies for domain into the export file.
func WriteExport(domain string, data json.RawMessage) error {
	path, err := ExportFilePath()
	if err != nil {
		return err
	}

	ef := &ExportFile{Cookies: make(map[string]json.RawMessage)}

	// Read existing file to merge
	if raw, err := os.ReadFile(path); err == nil {
		json.Unmarshal(raw, ef)
		if ef.Cookies == nil {
			ef.Cookies = make(map[string]json.RawMessage)
		}
	}

	ef.Timestamp = time.Now().Unix()
	if domain == "" {
		ef.Cookies["_all"] = data
	} else {
		ef.Cookies[domain] = data
	}

	out, err := json.MarshalIndent(ef, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal export file: %w", err)
	}

	return os.WriteFile(path, out, 0600)
}

// ReadExportCookies reads cookies for domain from the export file.
// Returns nil if the file doesn't exist or the domain entry is missing.
// maxAge is the maximum allowed age in seconds; 0 means no limit.
func ReadExportCookies(domain string, maxAge int) ([]CookiePair, error) {
	path, err := ExportFilePath()
	if err != nil {
		return nil, err
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var ef ExportFile
	if err := json.Unmarshal(raw, &ef); err != nil {
		return nil, err
	}

	if maxAge > 0 {
		age := time.Now().Unix() - ef.Timestamp
		if age > int64(maxAge) {
			return nil, fmt.Errorf("export file expired (%ds old, max %ds)", age, maxAge)
		}
	}

	data, ok := ef.Cookies[domain]
	if !ok {
		return nil, fmt.Errorf("domain %q not found in export file", domain)
	}

	var cookies []CookiePair
	if err := json.Unmarshal(data, &cookies); err != nil {
		return nil, fmt.Errorf("parse exported cookies: %w", err)
	}
	return cookies, nil
}

// ReadExportDomains reads all domain names from the export file.
func ReadExportDomains(maxAge int) ([]string, error) {
	path, err := ExportFilePath()
	if err != nil {
		return nil, err
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var ef ExportFile
	if err := json.Unmarshal(raw, &ef); err != nil {
		return nil, err
	}

	if maxAge > 0 {
		age := time.Now().Unix() - ef.Timestamp
		if age > int64(maxAge) {
			return nil, fmt.Errorf("export file expired (%ds old, max %ds)", age, maxAge)
		}
	}

	// If there's an _all key, try to extract unique domains from it
	if allData, ok := ef.Cookies["_all"]; ok {
		var cookies []struct {
			Domain string `json:"domain"`
		}
		if err := json.Unmarshal(allData, &cookies); err == nil {
			seen := make(map[string]bool)
			var domains []string
			for _, c := range cookies {
				d := c.Domain
				if len(d) > 0 && d[0] == '.' {
					d = d[1:]
				}
				if !seen[d] {
					seen[d] = true
					domains = append(domains, d)
				}
			}
			return domains, nil
		}
	}

	domains := make([]string, 0, len(ef.Cookies))
	for k := range ef.Cookies {
		if k != "_all" {
			domains = append(domains, k)
		}
	}
	return domains, nil
}
