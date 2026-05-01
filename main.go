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
	"path/filepath"
	"strings"
	"sync"
	"time"

	"xueya/internal/health"
)

//go:embed web/*
var webFiles embed.FS

var (
	appVersion = "0.1.5"
	gitCommit  = "unknown"
)

type apiServer struct {
	store    *health.Store
	cfg      health.Config
	cfgPath  string
	mu       sync.RWMutex
	sessions map[string]time.Time
}

func (s *apiServer) RLock()    {}
func (s *apiServer) RUnlock() {}

type apiServerReal struct {
	store    *health.Store
	cfg      health.Config
	cfgPath  string
	mu       sync.RWMutex
	sessions map[string]time.Time
}

func main() {
	seed := health.ParseRaw(health.RawData)

	cfgPath := os.Getenv("CONFIG_PATH")
	if cfgPath == "" {
		cfgPath = "config/config.json"
	}

	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o755); err != nil {
		log.Fatalf("create config directory: %v", err)
	}

	cfg, err := health.LoadConfig(cfgPath)
	if err != nil && !os.IsNotExist(err) {
		log.Printf("load config error: %v", err)
	}
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

	if err := os.MkdirAll(filepath.Dir(dataPath), 0o755); err != nil {
		log.Fatalf("create data directory: %v", err)
	}

	loginUser := strings.TrimSpace(os.Getenv("LOGIN_USER"))
	loginPass := os.Getenv("LOGIN_PASS")
	if loginUser != "" && loginPass != "" {
		if cfg.Username != loginUser || !cfg.CheckPassword(loginPass) {
			cfg.Username = loginUser
			if err := cfg.SetPassword(loginPass); err != nil {
				log.Fatalf("set password from env: %v", err)
			}
			log.Printf("configured user %s from environment", loginUser)
		}
	}

	store, err := health.NewStore(dataPath, seed.Records, seed.Issues)
	if err != nil {
		log.Fatalf("init store: %v", err)
	}

	if err := cfg.Save(cfgPath); err != nil {
		log.Fatalf("save initial config: %v", err)
	}
	log.Printf(
		"xueya version=%s commit=%s config=%s data=%s login_user_set=%t login_pass_set=%t has_password=%t username=%s",
		appVersion,
		gitCommit,
		cfgPath,
		dataPath,
		loginUser != "",
		loginPass != "",
		cfg.Password != "",
		cfg.Username,
	)

	api := &apiServerReal{
		store:    store,
		cfg:      cfg,
		cfgPath:  cfgPath,
		sessions: make(map[string]time.Time),
	}

	mux := http.NewServeMux()

	sub, _ := fs.Sub(webFiles, "web")
	fsrv := http.FileServer(http.FS(sub))

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || r.URL.Path == "/index.html" {
			fsrv.ServeHTTP(w, r)
			return
		}
		f, err := sub.Open(strings.TrimPrefix(r.URL.Path, "/"))
		if err == nil {
			f.Close()
			fsrv.ServeHTTP(w, r)
			return
		}
		fsrv.ServeHTTP(w, r)
	})

	mux.HandleFunc("/api/login", api.login)
	mux.HandleFunc("/api/logout", api.logout)
	mux.HandleFunc("/api/config", api.requireAuth(api.config))
	mux.HandleFunc("/api/records", api.requireAuth(api.records))
	mux.HandleFunc("/api/records/", api.requireAuth(api.recordByID))
	mux.HandleFunc("/api/recognize", api.requireAuth(api.recognize))
	mux.HandleFunc("/api/records.xlsx", api.requireAuth(api.exportXLSX))

	mux.HandleFunc("/_xueya/login", api.login)
	mux.HandleFunc("/_xueya/logout", api.logout)
	mux.HandleFunc("/_xueya/config", api.requireAuth(api.config))
	mux.HandleFunc("/_xueya/records", api.requireAuth(api.records))
	mux.HandleFunc("/_xueya/records/", api.requireAuth(api.recordByID))
	mux.HandleFunc("/_xueya/recognize", api.requireAuth(api.recognize))
	mux.HandleFunc("/_xueya/records.xlsx", api.requireAuth(api.exportXLSX))

	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":6644"
	}

	log.Printf("server listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}

func (s *apiServerReal) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.requestToken(r) == "" {
			writeError(w, http.StatusUnauthorized, "未登录")
			return
		}
		next(w, r)
	}
}

func (s *apiServerReal) requestToken(r *http.Request) string {
	if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		token := strings.TrimPrefix(auth, "Bearer ")
		s.mu.RLock()
		_, ok := s.sessions[token]
		s.mu.RUnlock()
		if ok {
			return token
		}
	}
	if cookie, err := r.Cookie("access_token"); err == nil {
		s.mu.RLock()
		_, ok := s.sessions[cookie.Value]
		s.mu.RUnlock()
		if ok {
			return cookie.Value
		}
	}
	if token := r.URL.Query().Get("access_token"); token != "" {
		s.mu.RLock()
		_, ok := s.sessions[token]
		s.mu.RUnlock()
		if ok {
			return token
		}
	}
	return ""
}

func (s *apiServerReal) login(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "不支持的方法")
		return
	}

	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "无效请求")
		return
	}

	s.mu.RLock()
	cfg := s.cfg
	s.mu.RUnlock()

	if cfg.Username != req.Username || !cfg.CheckPassword(req.Password) {
		writeError(w, http.StatusUnauthorized, "用户名或密码错误")
		return
	}

	token := generateToken()
	s.mu.Lock()
	s.sessions[token] = time.Now()
	s.mu.Unlock()

	http.SetCookie(w, &http.Cookie{
		Name:     "access_token",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		MaxAge:   86400 * 30,
	})

	writeJSON(w, http.StatusOK, map[string]string{"access_token": token})
}

func (s *apiServerReal) logout(w http.ResponseWriter, r *http.Request) {
	token := s.requestToken(r)
	if token != "" {
		s.mu.Lock()
		delete(s.sessions, token)
		s.mu.Unlock()
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "access_token",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})
	w.WriteHeader(http.StatusNoContent)
}

func generateToken() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func (s *apiServerReal) config(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	cfg := s.cfg
	cfgPath := s.cfgPath
	s.mu.RUnlock()

	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, cfg)
	case http.MethodPost:
		var newCfg struct {
			VisionURL   string `json:"vision_url"`
			VisionKey   string `json:"vision_key"`
			VisionModel string `json:"vision_model"`
			NewPassword string `json:"new_password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&newCfg); err != nil {
			writeError(w, http.StatusBadRequest, "无效请求")
			return
		}

		s.mu.Lock()
		s.cfg.VisionURL = newCfg.VisionURL
		s.cfg.VisionKey = newCfg.VisionKey
		s.cfg.VisionModel = newCfg.VisionModel
		if newCfg.NewPassword != "" {
			s.cfg.SetPassword(newCfg.NewPassword)
		}
		if err := s.cfg.Save(cfgPath); err != nil {
			s.mu.Unlock()
			writeError(w, http.StatusInternalServerError, "保存配置失败")
			return
		}
		s.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	default:
		writeError(w, http.StatusMethodNotAllowed, "不支持的方法")
	}
}

func (s *apiServerReal) records(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	store := s.store
	s.mu.RUnlock()

	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]any{
			"records": store.All(),
			"issues":  store.Issues(),
		})
	case http.MethodPost:
		var record health.Record
		if err := json.NewDecoder(r.Body).Decode(&record); err != nil {
			writeError(w, http.StatusBadRequest, "无效请求")
			return
		}
		log.Printf("[Records] 收到保存请求: Date=%s, Time=%s, Glucose=%v/%v", record.Date, record.Time, record.DynamicGlucose, record.FingerGlucose)
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

func (s *apiServerReal) exportXLSX(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	store := s.store
	s.mu.RUnlock()

	month := r.URL.Query().Get("month")
	records := store.All()
	if month != "" {
		var filtered []health.Record
		for _, rec := range records {
			if strings.HasPrefix(rec.Date, month) {
				filtered = append(filtered, rec)
			}
		}
		records = filtered
	}

	data, err := health.ExportXLSX(records)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "生成 Excel 失败")
		return
	}

	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", `attachment; filename="xueya_export.xlsx"`)
	w.Write(data)
}

func (s *apiServerReal) recordByID(w http.ResponseWriter, r *http.Request) {
	id := recordIDFromPath(r.URL.Path)
	if id == "" {
		writeError(w, http.StatusNotFound, "记录不存在")
		return
	}

	s.mu.RLock()
	store := s.store
	s.mu.RUnlock()

	switch r.Method {
	case http.MethodDelete:
		log.Printf("[Records] 收到删除请求: ID=%s", id)
		if err := store.Delete(id); err != nil {
			log.Printf("[Records] 删除失败: %v", err)
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		log.Printf("[Records] 删除成功: ID=%s", id)
		w.WriteHeader(http.StatusNoContent)
	case http.MethodPut:
		var record health.Record
		if err := json.NewDecoder(r.Body).Decode(&record); err != nil {
			writeError(w, http.StatusBadRequest, "无效请求")
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

func (s *apiServerReal) recognize(w http.ResponseWriter, r *http.Request) {
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
	log.Printf("[Recognize] 收到图片上传, 大小: %d 字节, 类型: %s", len(imageData), contentType)
	if !strings.HasPrefix(contentType, "image/") {
		writeError(w, http.StatusBadRequest, "文件不是有效图片")
		return
	}

	b64 := base64.StdEncoding.EncodeToString(imageData)
	dataURL := "data:" + contentType + ";base64," + b64

	select {
	case <-r.Context().Done():
		log.Printf("客户端已断开，取消识别")
		return
	default:
	}

	result, err := s.callVisionAPI(dataURL)
	if err != nil {
		log.Printf("拍照识别失败: %v", err)
		writeError(w, http.StatusInternalServerError, "识别失败: "+err.Error())
		return
	}

	log.Printf("拍照识别成功: %v", result)
	writeJSON(w, http.StatusOK, map[string]any{"records": result})
}

func (s *apiServerReal) callVisionAPI(imageDataURL string) ([]map[string]any, error) {
	s.mu.RLock()
	apiURL := s.cfg.VisionURL
	apiKey := s.cfg.VisionKey
	visionModel := s.cfg.VisionModel
	s.mu.RUnlock()

	log.Printf("[VisionAPI] 开始请求 AI 接口: %s, 模型: %s", apiURL, visionModel)

	if apiURL == "" || apiKey == "" {
		today := time.Now().Format("2006-01-02")
		result := []map[string]any{{
			"date":           today,
			"systolic":       120,
			"diastolic":      80,
			"pulse":          75,
			"dynamicGlucose": 5.5,
		}}
		log.Printf("拍照识别（演示模式）: %v", result)
		return result, nil
	}

	prompt := "你是一个精准的医疗数据提取助手。请仔细分析用户上传的健康仪器 LCD 屏幕照片，逐条提取屏幕中显示的全部测量记录。常见情况：血糖仪屏幕通常显示最近 3~7 天历史，每行包含日期（如 04/28）、时间（如 08:30）和血糖值；血压计屏幕通常仅显示当前一次测量。请严格返回一个 JSON 数组，每个元素是一个对象，键名用英文 camelCase：date (日期, YYYY-MM-DD 格式), time (时间, HH:MM 格式), systolic (收缩压, mmHg), diastolic (舒张压, mmHg), pulse (心率, bpm), dynamicGlucose (动态血糖, mmol/L), fingerGlucose (扎手指血糖, mmol/L)。尽量还原屏幕上的原始日期和时间。如某项无法读取则不输出该字段。禁止输出任何解释、注释或 markdown。"

	model := visionModel
	if model == "" {
		model = "Qwen/Qwen3.5-4B"
	}

	reqBody := map[string]any{
		"model": model,
		"messages": []map[string]any{
			{
				"role":    "system",
				"content": prompt,
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

	log.Printf("正在调用 Vision API (Model: %s, URL: %s)...", model, apiURL)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call API: %w", err)
	}
	defer resp.Body.Close()

	log.Printf("Vision API 响应状态: %d", resp.StatusCode)

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		log.Printf("[VisionAPI] 接口返回错误, 状态码: %d, 响应: %s", resp.StatusCode, string(respBody))
		return nil, fmt.Errorf("API status %d: %s", resp.StatusCode, string(respBody))
	}
	log.Printf("[VisionAPI] 接口响应成功")

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
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func recordIDFromPath(path string) string {
	for _, prefix := range []string{"/api/records/", "/_xueya/records/"} {
		if strings.HasPrefix(path, prefix) {
			return strings.TrimSpace(path[len(prefix):])
		}
	}
	if index := strings.LastIndex(path, "/"); index >= 0 && index < len(path)-1 {
		return strings.TrimSpace(path[index+1:])
	}
	return ""
}

