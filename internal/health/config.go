package health

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
)

type Config struct {
	Username    string `json:"username"`
	DataPath    string `json:"data_path"`
	Password    string `json:"password"`
	Salt        string `json:"salt"`
	VisionURL   string `json:"vision_url"`
	VisionKey   string `json:"vision_key"`
	VisionModel string `json:"vision_model"`
}

func LoadConfig(path string) (Config, error) {
	cfg := Config{}
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func (c Config) Save(path string) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	if dir := filepath.Dir(path); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return os.WriteFile(path, data, 0o644)
}

func (c Config) CheckPassword(password string) bool {
	if c.Password == "" || c.Salt == "" {
		return false
	}
	return c.Password == hashPassword(password, c.Salt)
}

func (c *Config) SetPassword(password string) error {
	saltBytes := make([]byte, 16)
	if _, err := rand.Read(saltBytes); err != nil {
		return err
	}
	salt := hex.EncodeToString(saltBytes)
	c.Salt = salt
	c.Password = hashPassword(password, salt)
	return nil
}

func hashPassword(password, salt string) string {
	hash := sha256.Sum256([]byte(salt + password))
	return hex.EncodeToString(hash[:])
}
