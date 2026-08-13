package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/amirarzideh/MikroTunnel/internal/domain"
	"github.com/amirarzideh/MikroTunnel/internal/provider/gre"
	"github.com/amirarzideh/MikroTunnel/internal/security"
	"github.com/amirarzideh/MikroTunnel/internal/store"
	"github.com/amirarzideh/MikroTunnel/internal/system"
)

type Server struct {
	store     domain.TunnelStore
	inspector system.Inspector
	logger    *slog.Logger
	providers map[domain.TunnelType]domain.TunnelProvider
}

func New(db domain.TunnelStore, inspector system.Inspector, logger *slog.Logger) *Server {
	p := gre.Provider{}
	return &Server{store: db, inspector: inspector, logger: logger, providers: map[domain.TunnelType]domain.TunnelProvider{p.Type(): p}}
}
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.health)
	mux.Handle("/api/v1/", s.requireKey(http.HandlerFunc(s.api)))
	return s.log(mux)
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "service": "mikrotunnel", "time": time.Now().UTC()})
}
func (s *Server) api(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1")
	switch {
	case r.Method == http.MethodGet && path == "/system":
		writeJSON(w, http.StatusOK, s.inspector.Status())
	case r.Method == http.MethodGet && path == "/tunnels":
		s.listTunnels(w, r)
	case r.Method == http.MethodPost && path == "/tunnels":
		s.createTunnel(w, r)
	case r.Method == http.MethodPost && strings.HasSuffix(path, "/enable"):
		s.setTunnelState(w, r, strings.TrimSuffix(strings.TrimPrefix(path, "/tunnels/"), "/enable"), domain.DesiredEnabled)
	case r.Method == http.MethodPost && strings.HasSuffix(path, "/disable"):
		s.setTunnelState(w, r, strings.TrimSuffix(strings.TrimPrefix(path, "/tunnels/"), "/disable"), domain.DesiredDisabled)
	case r.Method == http.MethodGet && strings.HasPrefix(path, "/tunnels/"):
		s.getTunnel(w, r, strings.TrimPrefix(path, "/tunnels/"))
	case r.Method == http.MethodPut && strings.HasPrefix(path, "/tunnels/"):
		s.updateTunnel(w, r, strings.TrimPrefix(path, "/tunnels/"))
	case r.Method == http.MethodDelete && strings.HasPrefix(path, "/tunnels/"):
		s.deleteTunnel(w, r, strings.TrimPrefix(path, "/tunnels/"))
	case r.Method == http.MethodGet && path == "/operations":
		s.listOperations(w, r)
	default:
		writeError(w, http.StatusNotFound, "route not found")
	}
}
func (s *Server) requireKey(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := security.Authenticate(r.Context(), s.store, r.Header.Get("Authorization")); err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		next.ServeHTTP(w, r)
	})
}
func (s *Server) listTunnels(w http.ResponseWriter, r *http.Request) {
	items, err := s.store.ListTunnels(r.Context())
	if err != nil {
		writeError(w, 500, "could not load tunnels")
		return
	}
	writeJSON(w, 200, map[string]any{"items": items})
}
func (s *Server) getTunnel(w http.ResponseWriter, r *http.Request, id string) {
	if strings.Contains(id, "/") {
		writeError(w, 404, "route not found")
		return
	}
	t, err := s.store.GetTunnel(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, 404, "tunnel not found")
		return
	}
	if err != nil {
		writeError(w, 500, "could not load tunnel")
		return
	}
	writeJSON(w, 200, t)
}
func (s *Server) createTunnel(w http.ResponseWriter, r *http.Request) {
	var input domain.TunnelInput
	if err := decode(w, r, &input); err != nil {
		return
	}
	p, ok := s.providers[input.Type]
	if !ok {
		writeError(w, 400, "unsupported tunnel type")
		return
	}
	if input.MTU == 0 {
		input.MTU = 1476
	}
	if input.TTL == 0 {
		input.TTL = 255
	}
	if err := p.Validate(input); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	now := time.Now().UTC()
	t := domain.Tunnel{ID: store.NewID(), Name: input.Name, Type: input.Type, Local: input.Local, Remote: input.Remote, Address: input.Address, MTU: input.MTU, TTL: input.TTL, Description: input.Description, DesiredState: domain.DesiredEnabled, ActualState: domain.ActualPending, CreatedAt: now, UpdatedAt: now}
	if err := s.store.CreateTunnel(r.Context(), t); err != nil {
		writeError(w, 409, "tunnel name already exists")
		return
	}
	s.operation(r, t.ID, "create_tunnel", domain.OperationQueued, "desired state saved; waiting for reconciliation")
	writeJSON(w, 201, t)
}
func (s *Server) updateTunnel(w http.ResponseWriter, r *http.Request, id string) {
	current, err := s.store.GetTunnel(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, 404, "tunnel not found")
		return
	}
	if err != nil {
		writeError(w, 500, "could not load tunnel")
		return
	}
	if current.DesiredState == domain.DesiredDeleted {
		writeError(w, http.StatusConflict, "tunnel removal is already queued")
		return
	}
	var input domain.TunnelInput
	if err := decode(w, r, &input); err != nil {
		return
	}
	p, ok := s.providers[input.Type]
	if !ok {
		writeError(w, 400, "unsupported tunnel type")
		return
	}
	if input.MTU == 0 {
		input.MTU = 1476
	}
	if input.TTL == 0 {
		input.TTL = 255
	}
	if err := p.Validate(input); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	current.Name = input.Name
	current.Type = input.Type
	current.Local = input.Local
	current.Remote = input.Remote
	current.Address = input.Address
	current.MTU = input.MTU
	current.TTL = input.TTL
	current.Description = input.Description
	current.ActualState = domain.ActualPending
	current.LastError = ""
	current.UpdatedAt = time.Now().UTC()
	if err := s.store.UpdateTunnel(r.Context(), current); err != nil {
		writeError(w, 409, "tunnel name already exists")
		return
	}
	s.operation(r, current.ID, "update_tunnel", domain.OperationQueued, "desired state updated; waiting for reconciliation")
	writeJSON(w, 200, current)
}
func (s *Server) deleteTunnel(w http.ResponseWriter, r *http.Request, id string) {
	t, err := s.store.GetTunnel(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, 404, "tunnel not found")
		return
	}
	if err != nil {
		writeError(w, 500, "could not load tunnel")
		return
	}
	if t.DesiredState == domain.DesiredDeleted {
		writeError(w, http.StatusConflict, "tunnel removal is already queued")
		return
	}
	t.DesiredState = domain.DesiredDeleted
	t.ActualState = domain.ActualPending
	t.LastError = ""
	t.UpdatedAt = time.Now().UTC()
	if err := s.store.UpdateTunnel(r.Context(), t); err != nil {
		writeError(w, 500, "could not queue tunnel removal")
		return
	}
	s.operation(r, id, "delete_tunnel", domain.OperationQueued, "tunnel removal queued")
	writeJSON(w, http.StatusAccepted, map[string]any{"accepted": true, "id": t.ID})
}
func (s *Server) setTunnelState(w http.ResponseWriter, r *http.Request, id string, state domain.DesiredState) {
	if id == "" || strings.Contains(id, "/") {
		writeError(w, 404, "route not found")
		return
	}
	t, err := s.store.GetTunnel(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, 404, "tunnel not found")
		return
	}
	if err != nil {
		writeError(w, 500, "could not load tunnel")
		return
	}
	if t.DesiredState == domain.DesiredDeleted {
		writeError(w, http.StatusConflict, "tunnel removal is already queued")
		return
	}
	t.DesiredState = state
	t.ActualState = domain.ActualPending
	t.LastError = ""
	t.UpdatedAt = time.Now().UTC()
	if err := s.store.UpdateTunnel(r.Context(), t); err != nil {
		writeError(w, 500, "could not update tunnel")
		return
	}
	s.operation(r, t.ID, string(state)+"_tunnel", domain.OperationQueued, "desired state updated; waiting for reconciliation")
	writeJSON(w, 200, t)
}
func (s *Server) listOperations(w http.ResponseWriter, r *http.Request) {
	items, err := s.store.ListOperations(r.Context(), 100)
	if err != nil {
		writeError(w, 500, "could not load operations")
		return
	}
	writeJSON(w, 200, map[string]any{"items": items})
}
func (s *Server) operation(r *http.Request, id, action string, status domain.OperationStatus, message string) {
	_ = s.store.CreateOperation(r.Context(), domain.Operation{ID: store.NewID(), Action: action, ResourceType: "tunnel", ResourceID: id, Status: status, Message: message, CreatedAt: time.Now().UTC()})
}
func (s *Server) log(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		next.ServeHTTP(w, r)
		s.logger.Info("http request", "method", r.Method, "path", r.URL.Path, "duration", time.Since(started))
	})
}
func decode(w http.ResponseWriter, r *http.Request, target any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	defer r.Body.Close()
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(target); err != nil {
		writeError(w, 400, "invalid JSON request")
		return err
	}
	return nil
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
