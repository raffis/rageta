package buildkitsetup

import (
	"strings"

	"github.com/moby/buildkit/client"
	"github.com/pkg/errors"
	"github.com/tonistiigi/go-csvvalue"
)

func parseExportCacheCSV(s string) (client.CacheOptionsEntry, error) {
	ex := client.CacheOptionsEntry{
		Type:  "",
		Attrs: map[string]string{},
	}
	fields, err := csvvalue.Fields(s, nil)
	if err != nil {
		return ex, err
	}

	for _, field := range fields {
		key, value, ok := strings.Cut(field, "=")
		if !ok {
			return ex, errors.Errorf("invalid value %s", field)
		}
		key = strings.ToLower(key)
		switch key {
		case "type":
			ex.Type = value
		default:
			ex.Attrs[key] = value
		}
	}
	if ex.Type == "" {
		return ex, errors.New("requires type=<type>")
	}
	if _, ok := ex.Attrs["mode"]; !ok {
		ex.Attrs["mode"] = "min"
	}
	if ex.Type == "gha" {
		return loadGithubEnv(ex)
	}
	return ex, nil
}

// ParseExportCache parses --export-cache
func ParseExportCache(exportCaches []string) ([]client.CacheOptionsEntry, error) {
	var exports []client.CacheOptionsEntry
	for _, exportCache := range exportCaches {
		ex, err := parseExportCacheCSV(exportCache)
		if err != nil {
			return nil, err
		}
		exports = append(exports, ex)
	}
	return exports, nil
}
