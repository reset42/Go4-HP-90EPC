package server

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"strconv"

	"hp90epc/assets"
	"hp90epc/logging"
	"hp90epc/model"
	"hp90epc/reader"
)

type App interface {
	GetLatest() *model.Measurement

	GetReaderStatus() reader.Status
	SetDevice(port string, baud int) error

	GetLogStatus() logging.LogStatus
	LogStart() (logging.LogStatus, error)
	LogStop() (logging.LogStatus, error)
	LogSetInterval(ms int) error
	LogListFiles() ([]string, error)
	LogReadFile(name string) ([]byte, error)
	LogTail(name string, maxLines int) ([]string, error)
}

func sendJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func Start(addr string, app App) error {
	mux := http.NewServeMux()

	// --- API: live
	mux.HandleFunc("/api/live", func(w http.ResponseWriter, r *http.Request) {
		// Wenn Reader nicht connected ist: kein "live"
		st := app.GetReaderStatus()
		if !st.Connected {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		m := app.GetLatest()
		if m == nil {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		sendJSON(w, m)
	})


	// --- API: reader status
	mux.HandleFunc("/api/reader/status", func(w http.ResponseWriter, r *http.Request) {
		sendJSON(w, app.GetReaderStatus())
	})

	// --- API: device port hot-swap
	mux.HandleFunc("/api/device/port", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Port string `json:"port"`
			Baud int    `json:"baud"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		if req.Port == "" {
			http.Error(w, "port required", http.StatusBadRequest)
			return
		}
		if req.Baud == 0 {
			req.Baud = 2400
		}
		if err := app.SetDevice(req.Port, req.Baud); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		sendJSON(w, app.GetReaderStatus())
	})

	// --- Logging API
	mux.HandleFunc("/api/log/status", func(w http.ResponseWriter, r *http.Request) {
		sendJSON(w, app.GetLogStatus())
	})
	mux.HandleFunc("/api/log/start", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		st, err := app.LogStart()
		if err != nil {
			http.Error(w, fmt.Sprintf("start logging: %v", err), http.StatusInternalServerError)
			return
		}
		sendJSON(w, st)
	})
	mux.HandleFunc("/api/log/stop", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		st, err := app.LogStop()
		if err != nil {
			http.Error(w, fmt.Sprintf("stop logging: %v", err), http.StatusInternalServerError)
			return
		}
		sendJSON(w, st)
	})
	mux.HandleFunc("/api/log/interval", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			IntervalMs int `json:"interval_ms"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		if req.IntervalMs <= 0 {
			req.IntervalMs = 1000
		}
		if err := app.LogSetInterval(req.IntervalMs); err != nil {
			http.Error(w, fmt.Sprintf("set interval: %v", err), http.StatusInternalServerError)
			return
		}
		sendJSON(w, app.GetLogStatus())
	})

	mux.HandleFunc("/api/log/files", func(w http.ResponseWriter, r *http.Request) {
		files, err := app.LogListFiles()
		if err != nil {
			http.Error(w, fmt.Sprintf("list files: %v", err), http.StatusInternalServerError)
			return
		}
		sendJSON(w, files)
	})

	mux.HandleFunc("/api/log/file", func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Query().Get("name")
		if name == "" {
			http.Error(w, "missing name", http.StatusBadRequest)
			return
		}
		data, err := app.LogReadFile(name)
		if err != nil {
			http.Error(w, fmt.Sprintf("read file: %v", err), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		_, _ = w.Write(data)
	})

	mux.HandleFunc("/api/log/tail", func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Query().Get("name")
		if name == "" {
			http.Error(w, "missing name", http.StatusBadRequest)
			return
		}
		n := 200
		if s := r.URL.Query().Get("lines"); s != "" {
			if v, err := strconv.Atoi(s); err == nil && v > 0 {
				n = v
			}
		}
		lines, err := app.LogTail(name, n)
		if err != nil {
			http.Error(w, fmt.Sprintf("tail file: %v", err), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		for _, line := range lines {
			io.WriteString(w, line)
			io.WriteString(w, "\n")
		}
	})

	// UI (embedded)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" && r.URL.Path != "/index.html" {
			http.NotFound(w, r)
			return
		}
		data, err := fs.ReadFile(assets.UI(), "index.html")
		if err != nil {
			http.Error(w, "index not found", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(data)
	})
	mux.HandleFunc("/hp90epc.css", func(w http.ResponseWriter, r *http.Request) {
		data, err := fs.ReadFile(assets.UI(), "hp90epc.css")
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		_, _ = w.Write(data)
	})

	log.Printf("HTTP server listening on %s", addr)
	return http.ListenAndServe(addr, mux)
}

