// Package docs menyediakan spesifikasi OpenAPI yang di-embed ke dalam binary.
package docs

import _ "embed"

// OpenAPI berisi isi file openapi.yaml.
//
//go:embed openapi.yaml
var OpenAPI []byte
