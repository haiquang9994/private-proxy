// Package caddyfile renders a Caddyfile from the list of enabled routes.
package caddyfile

import (
	"strings"
	"text/template"

	"private-proxy/internal/routes"
)

const tpl = `{{range .}}{{.Hostname}} {
    reverse_proxy {{.Upstream}} {
        transport http {
            keepalive 2m
            keepalive_idle_conns 10
        }
    }
}
{{end}}`

var parsedTpl = template.Must(template.New("caddyfile").Parse(tpl))

// Render generates Caddyfile contents for all enabled routes in s.
// Disabled routes are skipped entirely.
func Render(s *routes.Store) (string, error) {
	enabled := make([]routes.Route, 0, len(s.Routes))
	for _, r := range s.Routes {
		if r.Enabled {
			enabled = append(enabled, r)
		}
	}

	var sb strings.Builder
	if err := parsedTpl.Execute(&sb, enabled); err != nil {
		return "", err
	}
	return sb.String(), nil
}
