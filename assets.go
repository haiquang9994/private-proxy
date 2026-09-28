// Package privateproxy exposes repository files that proxyctl embeds, so a
// release binary can set up a server without a clone of this repo.
package privateproxy

import _ "embed"

// ComposeFile is docker-compose.yml, written into the project root by
// `proxyctl init`.
//
//go:embed docker-compose.yml
var ComposeFile []byte
