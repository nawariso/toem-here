package http_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nawariso/toem-hia/services/api/internal/domain"
	httptransport "github.com/nawariso/toem-hia/services/api/internal/transport/http"
)

func observedServer(t *testing.T) (http.Handler, *bytes.Buffer) {
	t.Helper()
	buffer := &bytes.Buffer{}
	logger := slog.New(slog.NewJSONHandler(buffer, nil))
	username, name := "mickey", "Mickey"
	user := domain.User{ID: "internal-id", Username: &username, DisplayName: &name, Locale: "th", Status: "ACTIVE", Roles: []string{"USER"}}
	return httptransport.NewServer(app{user: user}, verifier{}, ready{}, httptransport.WithLogger(logger)).Handler(), buffer
}

func logLine(t *testing.T, buffer *bytes.Buffer) map[string]any {
	t.Helper()
	line := strings.TrimSpace(buffer.String())
	if line == "" {
		t.Fatal("no structured log line was written")
	}
	entry := map[string]any{}
	if err := json.Unmarshal([]byte(line), &entry); err != nil {
		t.Fatalf("log line is not structured JSON: %v (%q)", err, line)
	}
	return entry
}

func TestRequestLogCarriesRequestIDMethodPathStatusAndDuration(t *testing.T) {
	handler, buffer := observedServer(t)
	req := httptest.NewRequest(http.MethodGet, "/v1/users/me", nil)
	req.Header.Set("Authorization", "Bearer valid")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	entry := logLine(t, buffer)
	requestID, _ := entry["request_id"].(string)
	if requestID == "" {
		t.Fatalf("structured log must carry the request id, got %v", entry)
	}
	if header := rec.Header().Get("X-Request-ID"); header != requestID {
		t.Fatalf("log request id %q must match response header %q", requestID, header)
	}
	if entry["method"] != http.MethodGet || entry["path"] != "/v1/users/me" || entry["status"] != float64(http.StatusOK) {
		t.Fatalf("log is missing required request fields: %v", entry)
	}
	if _, ok := entry["duration_ms"]; !ok {
		t.Fatalf("log is missing duration_ms: %v", entry)
	}
}

func TestRequestLogNeverContainsCredentialsOrTokens(t *testing.T) {
	handler, buffer := observedServer(t)
	req := httptest.NewRequest(http.MethodPatch, "/v1/users/me", strings.NewReader(`{"username":"mickey","displayName":"Mickey"}`))
	req.Header.Set("Authorization", "Bearer valid")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	logged := buffer.String()
	for _, secret := range []string{"Bearer", "valid", "Authorization", "person@example.com"} {
		if strings.Contains(logged, secret) {
			t.Fatalf("log leaked %q: %s", secret, logged)
		}
	}
}
