package buildkitsetup

import (
	"strings"

	"github.com/moby/buildkit/client"
	"github.com/pkg/errors"
	"github.com/tonistiigi/go-csvvalue"
)

func parseImportCacheCSV(s string) (client.CacheOptionsEntry, error) {
	im := client.CacheOptionsEntry{
		Type:  "",
		Attrs: map[string]string{},
	}
	fields, err := csvvalue.Fields(s, nil)
	if err != nil {
		return im, err
	}
	for _, field := range fields {
		key, value, ok := strings.Cut(field, "=")
		if !ok {
			return im, errors.Errorf("invalid value %s", field)
		}
		key = strings.ToLower(key)
		switch key {
		case "type":
			im.Type = value
		default:
			im.Attrs[key] = value
		}
	}
	if im.Type == "" {
		return im, errors.New("--import-cache requires type=<type>")
	}
	if im.Type == "gha" {
		return loadGithubEnv(im)
	}
	return im, nil
}

// ParseImportCache parses --import-cache
func ParseImportCache(importCaches []string) ([]client.CacheOptionsEntry, error) {
	var imports []client.CacheOptionsEntry
	for _, importCache := range importCaches {
		im, err := parseImportCacheCSV(importCache)
		if err != nil {
			return nil, err
		}
		imports = append(imports, im)
	}
	return imports, nil
}
