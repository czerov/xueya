package main

import (
	"bytes"
	"crypto/rand"
	"embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"xueya/internal/health"
)

//go:embed web/*
var webFiles embed.FS

type apiServer struct {
	mu        sync.RWMutex
	store     *health.Store
	cfg       health.Config
	cfgPath   string
	sessions  map[string]time.Time
	sessionMu sync.Mutex
}

func main() {
	seed := health.ParseRaw(health.RawData)

	cfgPath := os.Getenv("CONFIG_PATH")
	if cfgPath == "" {
		cfgPath = "config/config.json"
	}
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

	api := &apiServer{
		store:    store,
		cfg:      cfg,
		cfgPath:  cfgPath,
		sessions: make(map[string]time.Time),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/login", api.login)
	mux.HandleFunc("/api/logout", api.logout)
	mux.HandleFunc("/api/check", api.check)
	mux.HandleFunc("/api/config", api.auth(api.config))
	mux.HandleFunc("/api/records", api.auth(api.records))
	mux.HandleFunc("/api/records.xlsx", api.auth(api.recordsXLSX))
	mux.HandleFunc("/api/records/", api.auth(api.recordByID))
	mux.HandleFunc("/api/recognize", api.auth(api.recognize))
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

func (s *apiServer) check(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	hasPassword := s.cfg.Password != ""
	s.mu.RUnlock()
	writeJSON(w, http.StatusOK, map[string]any{
		"has_password": hasPassword,
		"authed":       s.validSession(r),
	})
}

func (s *apiServer) login(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "不支持的方法")
		return
	}

	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "请求数据不是有效 JSON")
		return
	}

	s.mu.RLock()
	hasPassword := s.cfg.Password != ""
	cfg := s.cfg
	s.mu.RUnlock()

	if !hasPassword {
		s.mu.Lock()
		if err := cfg.SetPassword(req.Password); err != nil {
			s.mu.Unlock()
			writeError(w, http.StatusInternalServerError, "设置密码失败")
			return
		}
		s.cfg = cfg
		s.mu.Unlock()
		if err := cfg.Save(s.cfgPath); err != nil {
			log.Printf("save config: %v", err)
		}
	} else if !cfg.CheckPassword(req.Password) {
		writeError(w, http.StatusUnauthorized, "密码错误")
		return
	}

	token := s.newSession()
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   86400 * 7,
	})
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

func (s *apiServer) logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie("session"); err == nil {
		s.sessionMu.Lock()
		delete(s.sessions, cookie.Value)
		s.sessionMu.Unlock()
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
	})
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

func (s *apiServer) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.validSession(r) {
			writeError(w, http.StatusUnauthorized, "未登录")
			return
		}
		next(w, r)
	}
}

func (s *apiServer) validSession(r *http.Request) bool {
	cookie, err := r.Cookie("session")
	if err != nil {
		return false
	}
	s.sessionMu.Lock()
	expiry, ok := s.sessions[cookie.Value]
	if ok && time.Now().Before(expiry) {
		s.sessionMu.Unlock()
		return true
	}
	delete(s.sessions, cookie.Value)
	s.sessionMu.Unlock()
	return false
}

func (s *apiServer) newSession() string {
	b := make([]byte, 32)
	rand.Read(b)
	token := hex.EncodeToString(b)
	s.sessionMu.Lock()
	s.sessions[token] = time.Now().Add(7 * 24 * time.Hour)
	s.sessionMu.Unlock()
	return token
}

func (s *apiServer) config(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.mu.RLock()
		defer s.mu.RUnlock()
		writeJSON(w, http.StatusOK, map[string]string{
			"data_path": s.cfg.DataPath,
		})
	case http.MethodPost:
		var req struct {
			DataPath     string `json:"data_path"`
			NewPassword  string `json:"new_password"`
			OldPassword  string `json:"old_password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "请求数据不是有效 JSON")
			return
		}

		s.mu.Lock()
		defer s.mu.Unlock()

		if req.NewPassword != "" {
			if s.cfg.Password != "" && !s.cfg.CheckPassword(req.OldPassword) {
				writeError(w, http.StatusBadRequest, "原密码错误")
				return
			}
			if err := s.cfg.SetPassword(req.NewPassword); err != nil {
				writeError(w, http.StatusInternalServerError, "设置密码失败")
				return
			}
		}

		if req.DataPath != "" && req.DataPath != s.cfg.DataPath {
			newStore, err := health.NewStore(req.DataPath, nil, nil)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "无法打开新数据文件: "+err.Error())
				return
			}
			s.store = newStore
			s.cfg.DataPath = req.DataPath
		}

		if err := s.cfg.Save(s.cfgPath); err != nil {
			writeError(w, http.StatusInternalServerError, "保存配置失败: "+err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{
			"data_path": s.cfg.DataPath,
		})
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

func (s *apiServer) recognize(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "不支持的方法")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 16<<20)
	if err := r.ParseMultipartForm(16 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "图片过大或格式不正确")
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "请选择图片")
		return
	}
	defer file.Close()

	imageData, err := io.ReadAll(io.LimitReader(file, 16<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, "读取图片失败")
		return
	}

	contentType := http.DetectContentType(imageData)
	if !strings.HasPrefix(contentType, "image/") {
		writeError(w, http.StatusBadRequest, "文件不是有效图片")
		return
	}

	b64 := base64.StdEncoding.EncodeToString(imageData)
	dataURL := "data:" + contentType + ";base64," + b64

	result, err := callVisionAPI(dataURL)
	if err != nil {
		log.Printf("拍照识别失败: %v", err)
		writeError(w, http.StatusInternalServerError, "识别失败: "+err.Error())
		return
	}

	log.Printf("拍照识别成功: %v", result)
	writeJSON(w, http.StatusOK, map[string]any{"records": result})
}

func callVisionAPI(imageDataURL string) ([]map[string]any, error) {
	apiURL := strings.TrimSpace(os.Getenv("VISION_API_URL"))
	apiKey := strings.TrimSpace(os.Getenv("VISION_API_KEY"))

	if apiURL == "" || apiKey == "" {
		result := []map[string]any{{
			"systolic":       120,
			"diastolic":      80,
			"pulse":          75,
			"dynamicGlucose": 5.5,
		}}
		log.Printf("拍照识别（演示模式）: %v", result)
		return result, nil
	}

	systemPrompt := "你是一个精准的医疗数据提取助手。请仔细分析用户上传的健康仪器 LCD 屏幕照片，逐条提取屏幕中显示的全部测量记录。常见情况：血糖仪屏幕通常显示最近 3~7 天历史，每行包含日期（如 04/28）、时间（如 08:30）和血糖值；血压计屏幕通常仅显示当前一次测量。请严格返回一个 JSON 数组，每个元素是一个对象，键名用英文 camelCase：date (日期, YYYY-MM-DD 格式), time (时间, HH:MM 格式), systolic (收缩压, mmHg), diastolic (舒张压, mmHg), pulse (心率, bpm), dynamicGlucose (动态血糖, mmol/L), fingerGlucose (扎手指血糖, mmol/L)。尽量还原屏幕上的原始日期和时间。如某项无法读取则不输出该字段。禁止输出任何解释、注释或 markdown。"

	model := strings.TrimSpace(os.Getenv("VISION_MODEL"))
	if model == "" {
		model = "Qwen/Qwen3.5-4B"
	}

	reqBody := map[string]any{
		"model": model,
		"messages": []map[string]any{
			{
				"role":    "system",
				"content": systemPrompt,
			},
			{
				"role": "user",
				"content": []map[string]any{
					{
						"type": "image_url",
						"image_url": map[string]string{
							"url": imageDataURL,
						},
					},
				},
			},
		},
		"max_tokens":      600,
		"temperature":     0,
		"enable_thinking": false,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, apiURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call API: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API status %d: %s", resp.StatusCode, string(respBody))
	}

	var openAIResp struct {
		Choices []struct {
			Message struct {
				Content          string `json:"content"`
				ReasoningContent string `json:"reasoning_content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &openAIResp); err != nil {
		return nil, fmt.Errorf("parse API response: %w", err)
	}

	if len(openAIResp.Choices) == 0 {
		return nil, errors.New("API 未返回有效内容")
	}

	content := openAIResp.Choices[0].Message.Content
	if content == "" {
		content = openAIResp.Choices[0].Message.ReasoningContent
	}
	content = extractJSON(content)
	log.Printf("识别原始返回: %s", content)

	if content == "" {
		return nil, errors.New("API 返回内容为空")
	}

	var result []map[string]any
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		var single map[string]any
		if err2 := json.Unmarshal([]byte(content), &single); err2 != nil {
			return nil, fmt.Errorf("parse result JSON: %w", err)
		}
		result = []map[string]any{single}
	}

	if len(result) == 0 {
		return nil, errors.New("未识别到任何数值")
	}

	return result, nil
}

func extractJSON(s string) string {
	s = strings.TrimSpace(s)
	if start := strings.Index(s, "```json"); start >= 0 {
		s = s[start+7:]
	} else if start := strings.Index(s, "```"); start >= 0 {
		s = s[start+3:]
	}
	if end := strings.LastIndex(s, "```"); end >= 0 {
		s = s[:end]
	}
	return strings.TrimSpace(s)
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
