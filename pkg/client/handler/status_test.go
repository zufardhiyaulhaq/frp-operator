package handler

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"

	"github.com/zufardhiyaulhaq/frp-operator/pkg/client/models"
)

const statusBody = `{
  "http": [
    {"name": "web", "type": "http", "status": "running", "err": "", "local_addr": "10.0.0.5:80", "plugin": "", "remote_addr": "web.example.com"}
  ],
  "tcp": [
    {"name": "ssh", "type": "tcp", "status": "start error", "err": "port already used", "local_addr": "10.0.0.6:22", "plugin": "", "remote_addr": ":6000"},
    {"name": "db", "type": "tcp", "status": "running", "err": "", "local_addr": "10.0.0.7:5432", "plugin": "", "remote_addr": ":6001"}
  ]
}`

func statusServer(t *testing.T, handler http.HandlerFunc) models.Config {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	u, _ := url.Parse(srv.URL)
	port, _ := strconv.Atoi(u.Port())
	return models.Config{Common: models.Common{
		AdminAddress:  u.Hostname(),
		AdminPort:     port,
		AdminUsername: "user",
		AdminPassword: "pass",
	}}
}

func TestStatus_FlattensAndSorts(t *testing.T) {
	var gotAuth string
	cfg := statusServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		if r.URL.Path != "/api/status" {
			t.Errorf("path = %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(statusBody))
	})

	got, err := Status(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotAuth != "Basic dXNlcjpwYXNz" { // base64("user:pass")
		t.Errorf("auth header = %q", gotAuth)
	}
	wantNames := []string{"db", "ssh", "web"}
	if len(got) != len(wantNames) {
		t.Fatalf("len = %d, want %d", len(got), len(wantNames))
	}
	for i, n := range wantNames {
		if got[i].Name != n {
			t.Errorf("got[%d].Name = %q, want %q", i, got[i].Name, n)
		}
	}
	if got[1].Status != "start error" || got[1].Err != "port already used" || got[1].RemoteAddr != ":6000" || got[1].Type != "tcp" {
		t.Errorf("ssh entry mismatch: %+v", got[1])
	}
	if got[2].LocalAddr != "10.0.0.5:80" || got[2].RemoteAddr != "web.example.com" {
		t.Errorf("web entry mismatch: %+v", got[2])
	}
}

func TestStatus_EmptyResponse(t *testing.T) {
	cfg := statusServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	})
	got, err := Status(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("len = %d, want 0", len(got))
	}
}

func TestStatus_Non200(t *testing.T) {
	cfg := statusServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("unauthorized"))
	})
	if _, err := Status(cfg); err == nil {
		t.Fatal("expected error for 401")
	}
}

func TestStatus_BadJSON(t *testing.T) {
	cfg := statusServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`not json`))
	})
	if _, err := Status(cfg); err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestStatus_NoAdminPort(t *testing.T) {
	if _, err := Status(models.Config{}); err == nil {
		t.Fatal("expected error when admin port is 0")
	}
}
