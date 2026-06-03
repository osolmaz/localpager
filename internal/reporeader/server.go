package reporeader

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Server struct {
	manager Manager

	mu              sync.Mutex
	bindings        map[string]Binding
	toolCalls       int64
	policyDenials   int64
	truncatedOutput int64
}

type BindRequest struct {
	DefaultRepo  string   `json:"default_repo"`
	VisibleRepos []string `json:"visible_repos"`
}

type BindResponse struct {
	RunID        string            `json:"run_id"`
	CWD          string            `json:"cwd"`
	DefaultRepo  string            `json:"default_repo"`
	VisibleRepos []string          `json:"visible_repos"`
	Snapshots    map[string]string `json:"snapshots"`
}

type ServiceExecRequest struct {
	RunID   string `json:"run_id"`
	Command string `json:"command"`
}

type StatusResponse struct {
	ActiveRuns      int               `json:"active_runs"`
	ToolCalls       int64             `json:"tool_calls"`
	PolicyDenials   int64             `json:"policy_denials"`
	TruncatedOutput int64             `json:"truncated_output"`
	Runs            map[string]string `json:"runs"`
}

func NewServer(manager Manager) *Server {
	return &Server{manager: manager, bindings: map[string]Binding{}}
}

func (s *Server) ServeUnix(ctx context.Context, socketPath string) error {
	socketPath, err := expandHome(socketPath)
	if err != nil {
		return err
	}
	if socketPath == "" {
		return fmt.Errorf("repo reader socket path is required")
	}
	if err := os.MkdirAll(filepath.Dir(socketPath), 0o755); err != nil {
		return fmt.Errorf("create socket dir: %w", err)
	}
	_ = os.Remove(socketPath)
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", socketPath, err)
	}
	defer func() { _ = os.Remove(socketPath) }()
	server := &http.Server{Handler: s.handler()}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()
	if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func (s *Server) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/bind", s.handleBind)
	mux.HandleFunc("/exec", s.handleExec)
	mux.HandleFunc("/status", s.handleStatus)
	return mux
}

func (s *Server) handleBind(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req BindRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid JSON: %v", err))
		return
	}
	binding, err := s.manager.Bind(r.Context(), req.DefaultRepo, req.VisibleRepos)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	runID, err := randomID()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.mu.Lock()
	s.bindings[runID] = binding
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, BindResponse{
		RunID:        runID,
		CWD:          binding.CWD,
		DefaultRepo:  binding.DefaultRepo,
		VisibleRepos: binding.VisibleRepos,
		Snapshots:    binding.Snapshots,
	})
}

func (s *Server) handleExec(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req ServiceExecRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid JSON: %v", err))
		return
	}
	s.mu.Lock()
	binding, ok := s.bindings[req.RunID]
	s.mu.Unlock()
	if !ok {
		writeError(w, http.StatusNotFound, "run binding not found")
		return
	}
	result := s.manager.Exec(r.Context(), ExecRequest{Command: req.Command, Binding: binding})
	s.mu.Lock()
	s.toolCalls++
	if result.PolicyError != "" {
		s.policyDenials++
	}
	if result.Truncated {
		s.truncatedOutput++
	}
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	runs := map[string]string{}
	for id, binding := range s.bindings {
		runs[id] = binding.CWD
	}
	writeJSON(w, http.StatusOK, StatusResponse{
		ActiveRuns:      len(s.bindings),
		ToolCalls:       s.toolCalls,
		PolicyDenials:   s.policyDenials,
		TruncatedOutput: s.truncatedOutput,
		Runs:            runs,
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func randomID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}
