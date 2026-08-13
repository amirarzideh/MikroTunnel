package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/amirarzideh/MikroTunnel/internal/domain"
	"github.com/amirarzideh/MikroTunnel/internal/security"
	"github.com/amirarzideh/MikroTunnel/internal/store"
	"github.com/amirarzideh/MikroTunnel/internal/system"
)

func TestAPIRequiresKeyAndCreatesDesiredTunnel(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db, err := store.Open(t.TempDir() + "/mikrotunnel.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	secret, err := security.Create(ctx, db)
	if err != nil {
		t.Fatal(err)
	}

	handler := New(db, system.Inspector{}, slog.New(slog.NewTextHandler(testWriter{t}, nil))).Handler()
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "MikroTunnel") {
		t.Fatalf("expected the dashboard shell, got %d", response.Code)
	}
	request = httptest.NewRequest(http.MethodGet, "/api/v1/tunnels", nil)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without a key, got %d", response.Code)
	}

	body := `{"name":"eu-gre-1","type":"gre","local_endpoint":"198.51.100.10","remote_endpoint":"203.0.113.20","address":"10.10.0.1/30","mtu":1476,"ttl":255}`
	request = httptest.NewRequest(http.MethodPost, "/api/v1/tunnels", strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer "+secret)
	request.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", response.Code, response.Body.String())
	}

	var tunnel domain.Tunnel
	if err := json.NewDecoder(response.Body).Decode(&tunnel); err != nil {
		t.Fatal(err)
	}
	if tunnel.ActualState != domain.ActualPending || tunnel.DesiredState != domain.DesiredEnabled {
		t.Fatalf("unexpected state: %#v", tunnel)
	}
	operations, err := db.ListOperations(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(operations) != 1 || operations[0].Status != domain.OperationQueued {
		t.Fatalf("expected one queued operation, got %#v", operations)
	}

	request = httptest.NewRequest(http.MethodDelete, "/api/v1/tunnels/"+tunnel.ID, nil)
	request.Header.Set("Authorization", "Bearer "+secret)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusAccepted {
		t.Fatalf("expected 202 for queued deletion, got %d: %s", response.Code, response.Body.String())
	}
	stored, err := db.GetTunnel(ctx, tunnel.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.DesiredState != domain.DesiredDeleted {
		t.Fatalf("expected a pending delete, got %q", stored.DesiredState)
	}
	operations, err = db.ListOperations(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(operations) != 2 || operations[0].Status != domain.OperationQueued {
		t.Fatalf("expected queued delete operation, got %#v", operations)
	}
}

type testWriter struct{ t *testing.T }

func (w testWriter) Write(p []byte) (int, error) {
	w.t.Log(strings.TrimSpace(string(p)))
	return len(p), nil
}
