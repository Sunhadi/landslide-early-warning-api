package httpapi

import (
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// specPath menunjuk ke dokumen OpenAPI.
func specPath(t *testing.T) string {
	t.Helper()
	// test berjalan di internal/httpapi, spec ada di docs/openapi.yaml (2 level di atas)
	p := filepath.Join("..", "..", "docs", "openapi.yaml")
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("spec tidak ditemukan: %v", err)
	}
	return p
}

// loadSpec membaca YAML OpenAPI menjadi map generik.
func loadSpec(t *testing.T) map[string]any {
	t.Helper()
	data, err := os.ReadFile(specPath(t))
	if err != nil {
		t.Fatalf("baca spec: %v", err)
	}
	var root map[string]any
	if err := yaml.Unmarshal(data, &root); err != nil {
		t.Fatalf("parse YAML: %v", err)
	}
	return root
}

func TestSpecValidYAMLAndVersion(t *testing.T) {
	root := loadSpec(t)
	if v, _ := root["openapi"].(string); !strings.HasPrefix(v, "3.1") {
		t.Fatalf("versi openapi = %q, mau 3.1.x", v)
	}
	info, ok := root["info"].(map[string]any)
	if !ok {
		t.Fatal("bagian info tidak ada")
	}
	if info["title"] == "" || info["version"] == "" {
		t.Fatal("info.title & info.version wajib ada")
	}
	if _, ok := root["paths"].(map[string]any); !ok {
		t.Fatal("bagian paths tidak ada")
	}
}

// pathsFromSpec mengumpulkan pasangan method+path dari spec.
func pathsFromSpec(root map[string]any) map[string]bool {
	out := map[string]bool{}
	paths, _ := root["paths"].(map[string]any)
	for p, raw := range paths {
		methods, _ := raw.(map[string]any)
		for m := range methods {
			mm := strings.ToUpper(m)
			if mm == "PARAMETERS" {
				continue
			}
			out[mm+" "+p] = true
		}
	}
	return out
}

// pathsFromRouter mengekstrak route dari kode router.go (parsing sederhana).
func pathsFromRouter(t *testing.T) map[string]bool {
	t.Helper()
	data, err := os.ReadFile("router.go")
	if err != nil {
		t.Fatalf("baca router.go: %v", err)
	}
	src := string(data)

	// cocokkan semua pemanggilan rute: r.Get(...), r.With(...).Post(...), dll.
	re := regexp.MustCompile(`\.(Get|Post|Patch|Delete|Put)\("([^"]+)"`)
	out := map[string]bool{}

	for _, m := range re.FindAllStringSubmatch(src, -1) {
		method := strings.ToUpper(m[1])
		path := m[2]
		// Lewati rute yang menyajikan dokumentasi itu sendiri.
		if path == "/openapi.yaml" || path == "/docs" {
			continue
		}
		prefix := "/v1"
		if path == "/health" || path == "/" {
			prefix = ""
		}
		out[method+" "+prefix+path] = true
	}
	return out
}

func TestSpecCoversRouterRoutes(t *testing.T) {
	root := loadSpec(t)
	spec := pathsFromSpec(root)
	router := pathsFromRouter(t)

	for route := range router {
		// normalisasi "GET /v1/auth/me" dst.
		if !spec[route] {
			t.Errorf("route %q ada di router tapi TIDAK ada di openapi.yaml", route)
		}
	}
}

func TestSpecHasNoUnknownRoutes(t *testing.T) {
	root := loadSpec(t)
	spec := pathsFromSpec(root)
	router := pathsFromRouter(t)

	for route := range spec {
		if !router[route] {
			t.Errorf("route %q ada di openapi.yaml tapi TIDAK ada di router", route)
		}
	}
}

func TestSpecHasSecurityScheme(t *testing.T) {
	root := loadSpec(t)
	comp, ok := root["components"].(map[string]any)
	if !ok {
		t.Fatal("components tidak ada")
	}
	sec, ok := comp["securitySchemes"].(map[string]any)
	if !ok {
		t.Fatal("securitySchemes tidak ada")
	}
	if _, ok := sec["bearerAuth"]; !ok {
		t.Fatal("securitySchemes.bearerAuth wajib ada")
	}
}

func TestSpecSchemasExist(t *testing.T) {
	root := loadSpec(t)
	comp := root["components"].(map[string]any)
	schemas := comp["schemas"].(map[string]any)
	required := []string{
		"Error", "PaginatedMeta", "Region", "RainfallData",
		"RiskAssessment", "RiskResponse", "Alert", "Report",
		"Subscription", "TokenPair", "AuditEntry",
	}
	keys := make([]string, 0, len(schemas))
	for k := range schemas {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, r := range required {
		if _, ok := schemas[r]; !ok {
			t.Errorf("schema %q tidak ada. Tersedia: %v", r, keys)
		}
	}
}

// --- Endpoint dokumentasi ---

func TestOpenAPISpecEndpoint(t *testing.T) {
	h := newHarness(t)
	rec := h.do(t, http.MethodGet, "/openapi.yaml", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct == "" {
		t.Error("Content-Type harus diset")
	}
	if rec.Body.Len() == 0 {
		t.Error("body spec kosong")
	}
	// spec harus memuat versi openapi
	if !strings.Contains(rec.Body.String(), "openapi:") {
		t.Error("body tidak seperti spec openapi")
	}
}

func TestSwaggerUIEndpoint(t *testing.T) {
	h := newHarness(t)
	rec := h.do(t, http.MethodGet, "/docs", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "swagger-ui") {
		t.Error("halaman docs harus memuat swagger UI")
	}
	if !strings.Contains(body, "/openapi.yaml") {
		t.Error("swagger harus menunjuk ke /openapi.yaml")
	}
}
