package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// ============================================================
// Configuration
// ============================================================

type Config struct {
	ListenAddr  string // API server listen address
	MediaMTXAPI string // MediaMTX Control API (internal Docker URL)
	WebRTCPort  string // MediaMTX WebRTC port (for browser URLs)
	HLSPort     string // MediaMTX HLS port
	RTSPPort    string // MediaMTX RTSP port
	StorePath   string // JSON file for stream persistence
	WebDir      string // Static web UI directory
}

func loadConfig() *Config {
	return &Config{
		ListenAddr:  env("LISTEN_ADDR", ":8080"),
		MediaMTXAPI: env("MEDIAMTX_API_URL", "http://localhost:9997"),
		WebRTCPort:  env("WEBRTC_PORT", "8889"),
		HLSPort:     env("HLS_PORT", "8888"),
		RTSPPort:    env("RTSP_PORT", "8554"),
		StorePath:   env("STORE_PATH", "./data/streams.json"),
		WebDir:      env("WEB_DIR", "./web"),
	}
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// ============================================================
// Models
// ============================================================

type Stream struct {
	Name      string `json:"name"`
	Label     string `json:"label"`
	RTSPUrl   string `json:"rtspUrl"`
	Status    string `json:"status"` // "connecting", "online", "offline"
	CreatedAt string `json:"createdAt"`
}

type AddStreamRequest struct {
	Name    string `json:"name"`    // optional, auto-generated if empty
	Label   string `json:"label"`   // optional, friendly name
	RTSPUrl string `json:"rtspUrl"` // required, RTSP source URL
}

type APIResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

// ============================================================
// Store — JSON file persistence
// ============================================================

type Store struct {
	mu      sync.RWMutex
	streams map[string]*Stream
	path    string
}

func NewStore(path string) *Store {
	s := &Store{
		streams: make(map[string]*Stream),
		path:    path,
	}
	s.load()
	return s
}

func (s *Store) load() {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return
	}
	var streams []*Stream
	if err := json.Unmarshal(data, &streams); err != nil {
		log.Printf("WARN: failed to parse store: %v", err)
		return
	}
	for _, st := range streams {
		s.streams[st.Name] = st
	}
	log.Printf("Loaded %d streams from %s", len(s.streams), s.path)
}

func (s *Store) save() {
	s.mu.RLock()
	streams := make([]*Stream, 0, len(s.streams))
	for _, st := range s.streams {
		streams = append(streams, st)
	}
	s.mu.RUnlock()

	data, _ := json.MarshalIndent(streams, "", "  ")
	dir := filepath.Dir(s.path)
	os.MkdirAll(dir, 0755)
	if err := os.WriteFile(s.path, data, 0644); err != nil {
		log.Printf("WARN: failed to save store: %v", err)
	}
}

func (s *Store) Add(stream *Stream) {
	s.mu.Lock()
	s.streams[stream.Name] = stream
	s.mu.Unlock()
	s.save()
}

func (s *Store) Delete(name string) {
	s.mu.Lock()
	delete(s.streams, name)
	s.mu.Unlock()
	s.save()
}

func (s *Store) Get(name string) *Stream {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.streams[name]
}

func (s *Store) List() []*Stream {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*Stream, 0, len(s.streams))
	for _, st := range s.streams {
		result = append(result, st)
	}
	return result
}

// ============================================================
// MediaMTX Client — Control API wrapper
// ============================================================

type MTXClient struct {
	apiURL string
	client *http.Client
}

func NewMTXClient(apiURL string) *MTXClient {
	return &MTXClient{
		apiURL: strings.TrimRight(apiURL, "/"),
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

// AddPath adds a new RTSP source path to MediaMTX
func (c *MTXClient) AddPath(name, source string) error {
	body, _ := json.Marshal(map[string]interface{}{
		"source":         source,
		"sourceOnDemand": false,
	})

	url := fmt.Sprintf("%s/v3/config/paths/add/%s", c.apiURL, name)
	req, _ := http.NewRequest("POST", url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("cannot reach MediaMTX API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("MediaMTX error %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

// DeletePath removes a path from MediaMTX
func (c *MTXClient) DeletePath(name string) error {
	url := fmt.Sprintf("%s/v3/config/paths/delete/%s", c.apiURL, name)
	req, _ := http.NewRequest("DELETE", url, nil)

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("cannot reach MediaMTX API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 && resp.StatusCode != 404 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("MediaMTX error %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

// PathInfo holds runtime path status from MediaMTX
type PathInfo struct {
	Name    string `json:"name"`
	Ready   bool   `json:"ready"`
	Source  *struct {
		Type string `json:"type"`
	} `json:"source"`
	Readers []interface{} `json:"readers"`
}

// ListPaths lists all active paths from MediaMTX
func (c *MTXClient) ListPaths() (map[string]PathInfo, error) {
	url := fmt.Sprintf("%s/v3/paths/list", c.apiURL)
	resp, err := c.client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Items []PathInfo `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	m := make(map[string]PathInfo, len(result.Items))
	for _, p := range result.Items {
		m[p.Name] = p
	}
	return m, nil
}

// Ping checks if MediaMTX API is reachable
func (c *MTXClient) Ping() error {
	_, err := c.ListPaths()
	return err
}

// ============================================================
// HTTP Server
// ============================================================

type Server struct {
	cfg   *Config
	store *Store
	mtx   *MTXClient
}

var nameRegex = regexp.MustCompile(`[^a-zA-Z0-9_-]`)

func sanitizeName(name string) string {
	name = strings.TrimSpace(strings.ToLower(name))
	name = strings.ReplaceAll(name, " ", "-")
	name = nameRegex.ReplaceAllString(name, "")
	if len(name) > 50 {
		name = name[:50]
	}
	return name
}

// POST /api/streams — add a new camera stream
func (s *Server) handleAddStream(w http.ResponseWriter, r *http.Request) {
	var req AddStreamRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "Invalid JSON body", http.StatusBadRequest)
		return
	}

	// Validate RTSP URL
	if req.RTSPUrl == "" {
		jsonError(w, "rtspUrl is required", http.StatusBadRequest)
		return
	}
	if !strings.HasPrefix(req.RTSPUrl, "rtsp://") && !strings.HasPrefix(req.RTSPUrl, "rtsps://") {
		jsonError(w, "URL must start with rtsp:// or rtsps://", http.StatusBadRequest)
		return
	}

	// Generate or sanitize name
	name := sanitizeName(req.Name)
	if name == "" {
		name = fmt.Sprintf("cam-%d", time.Now().UnixMilli())
	}

	// Check duplicate
	if s.store.Get(name) != nil {
		jsonError(w, fmt.Sprintf("Stream '%s' already exists", name), http.StatusConflict)
		return
	}

	// Add to MediaMTX via Control API
	if err := s.mtx.AddPath(name, req.RTSPUrl); err != nil {
		jsonError(w, err.Error(), http.StatusBadGateway)
		return
	}

	label := req.Label
	if label == "" {
		label = name
	}

	stream := &Stream{
		Name:      name,
		Label:     label,
		RTSPUrl:   req.RTSPUrl,
		Status:    "connecting",
		CreatedAt: time.Now().Format(time.RFC3339),
	}
	s.store.Add(stream)

	log.Printf("Added stream: %s → %s", name, req.RTSPUrl)
	jsonOK(w, stream)
}

// GET /api/streams — list all streams with live status
func (s *Server) handleListStreams(w http.ResponseWriter, r *http.Request) {
	streams := s.store.List()

	// Enrich with real-time status from MediaMTX
	if paths, err := s.mtx.ListPaths(); err == nil {
		for _, st := range streams {
			if p, ok := paths[st.Name]; ok {
				if p.Ready {
					st.Status = "online"
				} else {
					st.Status = "connecting"
				}
			} else {
				st.Status = "offline"
			}
		}
	}

	jsonOK(w, streams)
}

// DELETE /api/streams/{name} — remove a stream
func (s *Server) handleDeleteStream(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		jsonError(w, "Stream name is required", http.StatusBadRequest)
		return
	}

	if s.store.Get(name) == nil {
		jsonError(w, "Stream not found", http.StatusNotFound)
		return
	}

	// Remove from MediaMTX
	if err := s.mtx.DeletePath(name); err != nil {
		log.Printf("WARN: MediaMTX delete error: %v", err)
	}

	s.store.Delete(name)
	log.Printf("Deleted stream: %s", name)
	jsonOK(w, map[string]string{"deleted": name})
}

// GET /api/health — health check
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	mtxStatus := "ok"
	if err := s.mtx.Ping(); err != nil {
		mtxStatus = "unreachable"
	}
	jsonOK(w, map[string]interface{}{
		"status":   "ok",
		"mediamtx": mtxStatus,
		"ports": map[string]string{
			"webrtc": s.cfg.WebRTCPort,
			"hls":    s.cfg.HLSPort,
			"rtsp":   s.cfg.RTSPPort,
		},
	})
}

// GET /api/config — return port configuration for the web UI
func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, map[string]string{
		"webrtcPort": s.cfg.WebRTCPort,
		"hlsPort":    s.cfg.HLSPort,
		"rtspPort":   s.cfg.RTSPPort,
	})
}

// Restore streams from store to MediaMTX on startup
func (s *Server) restoreStreams() {
	streams := s.store.List()
	restored := 0
	for _, st := range streams {
		if err := s.mtx.AddPath(st.Name, st.RTSPUrl); err != nil {
			log.Printf("WARN: restore failed for %s: %v", st.Name, err)
		} else {
			restored++
		}
	}
	if len(streams) > 0 {
		log.Printf("Restored %d/%d streams to MediaMTX", restored, len(streams))
	}
}

// ============================================================
// Helpers
// ============================================================

func jsonOK(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(APIResponse{Success: true, Data: data})
}

func jsonError(w http.ResponseWriter, msg string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(APIResponse{Success: false, Error: msg})
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ============================================================
// Main
// ============================================================

func main() {
	cfg := loadConfig()
	store := NewStore(cfg.StorePath)
	mtx := NewMTXClient(cfg.MediaMTXAPI)

	srv := &Server{cfg: cfg, store: store, mtx: mtx}

	// Wait for MediaMTX then restore saved streams
	go func() {
		for i := 0; i < 30; i++ {
			if err := mtx.Ping(); err == nil {
				log.Println("MediaMTX API is reachable")
				srv.restoreStreams()
				return
			}
			time.Sleep(2 * time.Second)
		}
		log.Println("WARN: MediaMTX not reachable after 60s")
	}()

	mux := http.NewServeMux()

	// API routes (Go 1.22+ method routing)
	mux.HandleFunc("POST /api/streams", srv.handleAddStream)
	mux.HandleFunc("GET /api/streams", srv.handleListStreams)
	mux.HandleFunc("DELETE /api/streams/{name}", srv.handleDeleteStream)
	mux.HandleFunc("GET /api/health", srv.handleHealth)
	mux.HandleFunc("GET /api/config", srv.handleConfig)

	// Serve static web UI
	mux.Handle("/", http.FileServer(http.Dir(cfg.WebDir)))

	handler := corsMiddleware(mux)

	log.Println("============================================")
	log.Printf("  🚀 Stream API Server")
	log.Printf("  Listen:      %s", cfg.ListenAddr)
	log.Printf("  MediaMTX:    %s", cfg.MediaMTXAPI)
	log.Printf("  Web UI:      http://localhost%s", cfg.ListenAddr)
	log.Printf("  API:         http://localhost%s/api/streams", cfg.ListenAddr)
	log.Println("============================================")

	log.Fatal(http.ListenAndServe(cfg.ListenAddr, handler))
}
