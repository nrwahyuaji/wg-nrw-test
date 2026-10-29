package api

import (
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

// openAPIPath / openAPIMethod mengenali struktur `paths:` docs/openapi.yaml: path di indentasi 2, metode di 4.
// Sengaja tanpa pustaka YAML (tidak menambah dependensi langsung); formatnya dijaga oleh uji ini sendiri.
var (
	openAPIPath   = regexp.MustCompile(`^  (/[^:]*):\s*$`)
	openAPIMethod = regexp.MustCompile(`^    (get|post|put|delete|patch):`)
)

// TestOpenAPIMatchesRouter: setiap rute router ada di docs/openapi.yaml dan sebaliknya (PLAN 6.2: satu sumber
// kontrak). Gagal = kontrak dan kode menyimpang; perbarui keduanya di commit yang sama.
func TestOpenAPIMatchesRouter(t *testing.T) {
	raw, err := os.ReadFile("../../../docs/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	documented := map[string]bool{}
	current := ""
	inPaths := false
	for _, line := range strings.Split(string(raw), "\n") {
		switch {
		case line == "paths:":
			inPaths = true
		case inPaths && line != "" && !strings.HasPrefix(line, " "):
			inPaths = false
		case inPaths && openAPIPath.MatchString(line):
			current = openAPIPath.FindStringSubmatch(line)[1]
		case inPaths && current != "" && openAPIMethod.MatchString(line):
			documented[strings.ToUpper(openAPIMethod.FindStringSubmatch(line)[1])+" "+current] = true
		}
	}

	_, router, _ := newInterfaceTestApp(t)
	routed := map[string]bool{}
	walker, ok := router.(chi.Routes)
	if !ok {
		t.Fatal("router bukan chi.Routes")
	}
	err = chi.Walk(walker, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		routed[method+" "+strings.TrimSuffix(route, "/")] = true
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(documented) == 0 || len(routed) == 0 {
		t.Fatalf("kosong: dokumen %d rute %d", len(documented), len(routed))
	}
	t.Logf("%d operasi terdokumentasi, %d rute", len(documented), len(routed))
	for _, missing := range difference(routed, documented) {
		t.Errorf("rute tidak ada di openapi.yaml: %s", missing)
	}
	for _, extra := range difference(documented, routed) {
		t.Errorf("openapi.yaml memuat rute yang tidak ada di router: %s", extra)
	}
}

// difference mengembalikan kunci di a yang tidak ada di b, terurut.
func difference(a, b map[string]bool) []string {
	var result []string
	for key := range a {
		if !b[key] {
			result = append(result, key)
		}
	}
	sort.Strings(result)
	return result
}
