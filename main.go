package main

import (
	"embed"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"sync"

	"xueya/internal/health"
)

//go:embed web/*
var webFiles embed.FS

type apiServer struct {
	mu       sync.RWMutex
	store    *health.Store
	cfg      health.Config
	cfgPath  string
}

func main() {
	seed := health.ParseRaw(health.RawData)

	cfgPath := "config.json"
	cfg, err := health.LoadConfig(cfgPath)
	if err != nil {
		cfg = health.Config{}
	}

	dataPath := cfg.DataPath
	if dataPath == "" {
		dataPath = os.Getenv("DATA_PATH")
	}
	if dataPath == "" {
		dataPath = "data/records.json"
	}
	cfg.DataPath = dataPath

	store, err := health.NewStore(dataPath, seed.Records, seed.Issues)
	if err != nil {
		log.Fatalf("init store: %v", err)
	}

	if err := cfg.Save(cfgPath); err != nil {
		log.Printf("save config: %v", err)
	}

	webRoot, err := fs.Sub(webFiles, "web")
	if err != nil {
		log.Fatalf("load web files: %v", err)
	}

	api := &apiServer{store: store, cfg: cfg, cfgPath: cfgPath}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/records", api.records)
	mux.HandleFunc("/api/records.xlsx", api.recordsXLSX)
	mux.HandleFunc("/api/records/", api.recordByID)
	mux.HandleFunc("/api/config", api.config)
	mux.Handle("/", http.FileServer(http.FS(webRoot)))

	addr := env("ADDR", ":6644")
	log.Printf("blood glucose and pressure app listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("server: %v", err)
	}
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func (s *apiServer) config(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.mu.RLock()
		defer s.mu.RUnlock()
		writeJSON(w, http.StatusOK, s.cfg)
	case http.MethodPost:
		var cfg health.Config
		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			writeError(w, http.StatusBadRequest, "请求数据不是有效 JSON")
			return
		}
		dataPath := cfg.DataPath
		if dataPath == "" {
			writeError(w, http.StatusBadRequest, "data_path 不能为空")
			return
		}

		newStore, err := health.NewStore(dataPath, nil, nil)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "无法打开新数据文件: "+err.Error())
			return
		}

		cfg.DataPath = dataPath
		if err := cfg.Save(s.cfgPath); err != nil {
			writeError(w, http.StatusInternalServerError, "保存配置失败: "+err.Error())
			return
		}

		s.mu.Lock()
		s.store = newStore
		s.cfg = cfg
		s.mu.Unlock()

		writeJSON(w, http.StatusOK, s.cfg)
	default:
		writeError(w, http.StatusMethodNotAllowed, "不支持的方法")
	}
}

func (s *apiServer) records(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	store := s.store
	s.mu.RUnlock()

	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, health.Dataset{
			Records: store.All(),
			Issues:  store.Issues(),
		})
	case http.MethodPost:
		var record health.Record
		if err := json.NewDecoder(r.Body).Decode(&record); err != nil {
			writeError(w, http.StatusBadRequest, "请求数据不是有效 JSON")
			return
		}
		saved, err := store.Add(record)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, saved)
	default:
		writeError(w, http.StatusMethodNotAllowed, "不支持的方法")
	}
}

func (s *apiServer) recordsXLSX(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	store := s.store
	s.mu.RUnlock()

	switch r.Method {
	case http.MethodGet:
		data, err := health.ExportXLSX(filterRecords(store.All(), r))
		if err != nil {
			writeError(w, http.StatusInternalServerError, "导出 XLSX 失败")
			return
		}
		w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
		w.Header().Set("Content-Disposition", `attachment; filename="xueya-records.xlsx"`)
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write(data); err != nil {
			log.Printf("write xlsx: %v", err)
		}
	case http.MethodPost:
		r.Body = http.MaxBytesReader(w, r.Body, 16<<20)
		if err := r.ParseMultipartForm(16 << 20); err != nil {
			writeError(w, http.StatusBadRequest, "上传文件过大或格式不正确")
			return
		}
		file, _, err := r.FormFile("file")
		if err != nil {
			writeError(w, http.StatusBadRequest, "请选择 XLSX 文件")
			return
		}
		defer file.Close()

		data, err := io.ReadAll(io.LimitReader(file, 16<<20))
		if err != nil {
			writeError(w, http.StatusBadRequest, "读取 XLSX 文件失败")
			return
		}
		records, err := health.ImportXLSX(data)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		imported, skipped, err := store.ImportRecords(records)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]int{
			"imported": len(imported),
			"skipped":  skipped,
		})
	default:
		writeError(w, http.StatusMethodNotAllowed, "不支持的方法")
	}
}

func (s *apiServer) recordByID(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Path[len("/api/records/"):]
	if id == "" {
		writeError(w, http.StatusNotFound, "记录不存在")
		return
	}

	s.mu.RLock()
	store := s.store
	s.mu.RUnlock()

	switch r.Method {
	case http.MethodDelete:
		if err := store.Delete(id); err != nil {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case http.MethodPut:
		var record health.Record
		if err := json.NewDecoder(r.Body).Decode(&record); err != nil {
			writeError(w, http.StatusBadRequest, "请求数据不是有效 JSON")
			return
		}
		record.ID = id
		saved, err := store.Replace(record)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, saved)
	default:
		writeError(w, http.StatusMethodNotAllowed, "不支持的方法")
	}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("write response: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func filterRecords(records []health.Record, r *http.Request) []health.Record {
	query := r.URL.Query()
	month := query.Get("month")
	date := query.Get("date")
	segment := query.Get("segment")
	if month == "" && date == "" && (segment == "" || segment == "all") {
		return records
	}

	filtered := make([]health.Record, 0, len(records))
	for _, record := range records {
		if month != "" && !hasPrefix(record.Date, month) {
			continue
		}
		if date != "" && record.Date != date {
			continue
		}
		if segment != "" && segment != "all" && record.Segment != segment {
			continue
		}
		filtered = append(filtered, record)
	}
	return filtered
}

func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
