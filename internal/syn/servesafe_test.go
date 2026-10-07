package syn

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// A page on another site must not be able to make the dashboard push. The
// browser says where the request came from; anything that is not a browser
// says nothing and is left alone.
func TestDashboardRefusesRequestsFromOtherSites(t *testing.T) {
	reached := false
	h := sameOriginOnly(func(w http.ResponseWriter, r *http.Request) { reached = true })

	for _, tc := range []struct {
		name    string
		headers map[string]string
		allow   bool
	}{
		{"curl or a script", nil, true},
		{"our own page", map[string]string{"Sec-Fetch-Site": "same-origin", "Origin": "http://localhost:7391"}, true},
		{"typed into the address bar", map[string]string{"Sec-Fetch-Site": "none"}, true},
		{"a form on another website", map[string]string{"Sec-Fetch-Site": "cross-site", "Origin": "https://evil.example"}, false},
		{"an older browser, no fetch metadata", map[string]string{"Origin": "https://evil.example"}, false},
		{"same site, different port", map[string]string{"Origin": "http://localhost:9999"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reached = false
			req := httptest.NewRequest(http.MethodPost, "http://localhost:7391/api/push?repo=x", nil)
			req.Host = "localhost:7391"
			for k, v := range tc.headers {
				req.Header.Set(k, v)
			}
			rec := httptest.NewRecorder()
			h(rec, req)

			if reached != tc.allow {
				t.Errorf("handler reached = %v, want %v (status %d)", reached, tc.allow, rec.Code)
			}
			if !tc.allow && rec.Code != http.StatusForbidden {
				t.Errorf("status = %d, want 403", rec.Code)
			}
		})
	}
}

func TestListensBeyondThisMachine(t *testing.T) {
	for addr, want := range map[string]bool{
		"127.0.0.1:7391": false,
		"localhost:7391": false,
		"[::1]:7391":     false,
		":8080":          true,
		"0.0.0.0:8080":   true,
		"192.168.1.5:80": true,
	} {
		if got := listensBeyondThisMachine(addr); got != want {
			t.Errorf("listensBeyondThisMachine(%q) = %v, want %v", addr, got, want)
		}
	}
}
