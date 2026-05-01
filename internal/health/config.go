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
	TokenHash   string `json:"token_hash"`
	TokenSalt   string `json:"token_salt"`
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
	hash, salt, err := saltedHash(password)
	if err != nil {
		return err
	}
	c.Salt = salt
	c.Password = hash
	return nil
}

func (c Config) CheckSessionToken(token string) bool {
	if token == "" || c.TokenHash == "" || c.TokenSalt == "" {
		return false
	}
	return c.TokenHash == hashPassword(token, c.TokenSalt)
}

func (c *Config) SetSessionToken(token string) error {
	hash, salt, err := saltedHash(token)
	if err != nil {
		return err
	}
	c.TokenSalt = salt
	c.TokenHash = hash
	return nil
}

func (c *Config) ClearSessionToken() {
	c.TokenSalt = ""
	c.TokenHash = ""
}

func saltedHash(value string) (string, string, error) {
	saltBytes := make([]byte, 16)
	if _, err := rand.Read(saltBytes); err != nil {
		return "", "", err
	}
	salt := hex.EncodeToString(saltBytes)
	return hashPassword(value, salt), salt, nil
}

func hashPassword(password, salt string) string {
	hash := sha256.Sum256([]byte(salt + password))
	return hex.EncodeToString(hash[:])
}
