package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os/exec"
	"os"
	"strings"
	"time"

	"github.com/amirarzideh/MikroTunnel/internal/dashboard"
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
	apiKey    string
	username  string
	password  string
}

func New(db domain.TunnelStore, inspector system.Inspector, logger *slog.Logger, apiKey, username, password string) *Server {
	p := gre.Provider{}
	return &Server{store: db, inspector: inspector, logger: logger, providers: map[domain.TunnelType]domain.TunnelProvider{p.Type(): p}, apiKey: apiKey, username: username, password: password}
}
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.health)
	mux.Handle("/api/v1/", s.requireAPI(http.HandlerFunc(s.api)))
	mux.Handle("/", s.requireDashboard(dashboard.Handler()))
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
	case r.Method == http.MethodPost && strings.HasSuffix(path, "/probe"):
		s.probeTunnel(w, r, strings.TrimSuffix(strings.TrimPrefix(path, "/tunnels/"), "/probe"))
	case r.Method == http.MethodGet && strings.HasPrefix(path, "/tunnels/"):
		s.getTunnel(w, r, strings.TrimPrefix(path, "/tunnels/"))
	case r.Method == http.MethodPut && strings.HasPrefix(path, "/tunnels/"):
		s.updateTunnel(w, r, strings.TrimPrefix(path, "/tunnels/"))
	case r.Method == http.MethodDelete && strings.HasPrefix(path, "/tunnels/"):
		s.deleteTunnel(w, r, strings.TrimPrefix(path, "/tunnels/"))
	case r.Method == http.MethodGet && path == "/operations":
		s.listOperations(w, r)
	case r.Method == http.MethodGet && path == "/network/settings":
		s.networkSettings(w)
	case r.Method == http.MethodPut && path == "/network/ipv4-forward":
		s.setIPv4Forward(w, r)
	case r.Method == http.MethodGet && path == "/interfaces":
		s.listInterfaces(w)
	case r.Method == http.MethodDelete && strings.HasPrefix(path, "/interfaces/"):
		s.deleteInterface(w, strings.TrimPrefix(path, "/interfaces/"))
	default:
		writeError(w, http.StatusNotFound, "route not found")
	}
}
func (s *Server) listInterfaces(w http.ResponseWriter) {
	items, err := gre.Discover()
	if err != nil { writeError(w, 500, "could not discover GRE interfaces"); return }
	writeJSON(w, 200, map[string]any{"items": items})
}
func (s *Server) deleteInterface(w http.ResponseWriter, name string) {
	if name == "" || strings.ContainsAny(name, "/\\") { writeError(w, 400, "invalid interface name"); return }
	if err := gre.RemoveDiscovered(name); err != nil { writeError(w, 400, "could not remove GRE interface: "+err.Error()); return }
	writeJSON(w, 200, map[string]bool{"removed": true})
}
func (s *Server) networkSettings(w http.ResponseWriter) {
	value, err := os.ReadFile("/proc/sys/net/ipv4/ip_forward")
	if err != nil { writeError(w, 500, "could not read IPv4 forwarding state"); return }
	writeJSON(w, 200, map[string]bool{"ipv4_forward": strings.TrimSpace(string(value)) == "1"})
}
func (s *Server) setIPv4Forward(w http.ResponseWriter, r *http.Request) {
	var input struct { Enabled bool `json:"enabled"` }
	if err := decode(w, r, &input); err != nil { return }
	value := "0\n"; if input.Enabled { value = "1\n" }
	if err := os.WriteFile("/etc/sysctl.d/99-mikrotunnel-ip-forward.conf", []byte("net.ipv4.ip_forward="+strings.TrimSpace(value)+"\n"), 0o644); err != nil { writeError(w, 500, "could not persist IPv4 forwarding"); return }
	if err := os.WriteFile("/proc/sys/net/ipv4/ip_forward", []byte(value), 0o644); err != nil { writeError(w, 500, "could not apply IPv4 forwarding"); return }
	args := []string{"-C", "FORWARD", "-m", "comment", "--comment", "mikrotunnel:ipv4-forward", "-j", "ACCEPT"}
	if input.Enabled && exec.Command("iptables", args...).Run() != nil { args[0] = "-A"; if output, err := exec.Command("iptables", args...).CombinedOutput(); err != nil { writeError(w, 500, "could not allow tunnel forwarding: "+string(output)); return } }
	if !input.Enabled && exec.Command("iptables", args...).Run() == nil { args[0] = "-D"; if output, err := exec.Command("iptables", args...).CombinedOutput(); err != nil { writeError(w, 500, "could not remove tunnel forwarding rule: "+string(output)); return } }
	writeJSON(w, 200, map[string]bool{"ipv4_forward": input.Enabled})
}
func (s *Server) probeTunnel(w http.ResponseWriter, r *http.Request, id string) {
	t, err := s.store.GetTunnel(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) { writeError(w, 404, "tunnel not found"); return }
	if err != nil { writeError(w, 500, "could not load tunnel"); return }
	if t.DesiredState != domain.DesiredEnabled { writeError(w, 409, "enable the tunnel before probing"); return }
	target := r.URL.Query().Get("target")
	if target == "" { target = "1.1.1.1" }
	if net.ParseIP(target) == nil { writeError(w, 400, "probe target must be an IP address"); return }
	output, err := exec.CommandContext(r.Context(), "ping", "-I", t.Name, "-c", "3", "-W", "2", target).CombinedOutput()
	result := map[string]any{"tunnel_id": t.ID, "interface": t.Name, "target": target, "reachable": err == nil, "output": string(output)}
	if err != nil { writeJSON(w, http.StatusOK, result); return }
	writeJSON(w, http.StatusOK, result)
}
func (s *Server) requireAPI(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorization := r.Header.Get("Authorization")
		if err := security.AuthenticateStatic(s.apiKey, authorization); err != nil && security.AuthenticateDashboard(s.username, s.password, authorization) != nil && !s.hasDashboardSession(r) {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		next.ServeHTTP(w, r)
	})
}
func (s *Server) requireDashboard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := security.AuthenticateDashboard(s.username, s.password, r.Header.Get("Authorization")); err != nil {
			w.Header().Set("WWW-Authenticate", `Basic realm="MikroTunnel dashboard", charset="UTF-8"`)
			writeError(w, http.StatusUnauthorized, "dashboard login required")
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "mikrotunnel_session", Value: s.dashboardSession(), Path: "/", MaxAge: 43200, HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode})
		next.ServeHTTP(w, r)
	})
}

func (s *Server) dashboardSession() string {
	mac := hmac.New(sha256.New, []byte(s.apiKey))
	_, _ = mac.Write([]byte(s.username + ":" + s.password))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (s *Server) hasDashboardSession(r *http.Request) bool {
	cookie, err := r.Cookie("mikrotunnel_session")
	return err == nil && hmac.Equal([]byte(cookie.Value), []byte(s.dashboardSession()))
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
	if _, err := net.InterfaceByName(input.Name); err == nil {
		writeError(w, http.StatusConflict, "an interface with this name already exists; remove it or choose another name")
		return
	}
	now := time.Now().UTC()
	t := domain.Tunnel{ID: store.NewID(), Name: input.Name, Type: input.Type, Local: input.Local, Remote: input.Remote, Address: input.Address, MTU: input.MTU, TTL: input.TTL, Description: input.Description, Masquerade: input.Masquerade, DesiredState: domain.DesiredEnabled, ActualState: domain.ActualPending, CreatedAt: now, UpdatedAt: now}
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
	current.Masquerade = input.Masquerade
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
