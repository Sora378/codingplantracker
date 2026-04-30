package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/99designs/keyring"
)

const keyringService = "coplanage"

type Config struct {
	UserID          string    `json:"user_id"`
	Email           string    `json:"email"`
	Region          string    `json:"region"` // "global" or "china"
	ActiveProfileID string    `json:"active_profile_id,omitempty"`
	Profiles        []Profile `json:"profiles,omitempty"`

	path string `json:"-"`
}

type Profile struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Provider   string `json:"provider"`
	Region     string `json:"region,omitempty"`
	CodexHome  string `json:"codex_home,omitempty"`
	CreatedAt  string `json:"created_at,omitempty"`
	LastUsedAt string `json:"last_used_at,omitempty"`
}

type Credentials struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

func (c *Config) ConfigDir() string {
	if c.path != "" {
		return filepath.Dir(c.path)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".config/coplanage"
	}
	return filepath.Join(home, ".config", "coplanage")
}

func (c *Config) Path() string {
	if c.path != "" {
		return c.path
	}
	return filepath.Join(c.ConfigDir(), "config.json")
}

func (c *Config) keyring() (keyring.Keyring, error) {
	return keyring.Open(keyring.Config{
		ServiceName: keyringService,
	})
}

func (c *Config) ShowCredentialLocations() {
	fmt.Println("  📍 Credential Storage Locations:")
	fmt.Printf("  ✓ Config file: %s\n", c.Path())
	fmt.Printf("  ✓ Keyring service: %s\n", keyringService)
	if cwd, err := os.Getwd(); err == nil {
		fmt.Printf("  ✓ Working directory: %s (NO secrets here)\n", cwd)
	}
	fmt.Println()
	fmt.Println("  ✓ API key stored in OS keychain (encrypted, secure)")
}

func (c *Config) saveCredentials(cred *Credentials) error {
	kr, err := c.keyring()
	if err != nil {
		return err
	}

	data, err := json.Marshal(cred)
	if err != nil {
		return err
	}

	return kr.Set(keyring.Item{
		Key:  "credentials",
		Data: data,
	})
}

func (c *Config) loadCredentials() (*Credentials, error) {
	kr, err := c.keyring()
	if err != nil {
		if err == keyring.ErrKeyNotFound {
			return &Credentials{}, nil
		}
		return nil, err
	}

	item, err := kr.Get("credentials")
	if err != nil {
		if err == keyring.ErrKeyNotFound {
			return &Credentials{}, nil
		}
		return nil, err
	}

	var cred Credentials
	if err := json.Unmarshal(item.Data, &cred); err != nil {
		return nil, err
	}

	return &cred, nil
}

func (c *Config) ClearCredentials() error {
	kr, err := c.keyring()
	if err != nil {
		return err
	}
	if err := kr.Remove("credentials"); err != nil && err != keyring.ErrKeyNotFound {
		return err
	}
	return nil
}

func Load() (*Config, error) {
	return LoadFrom("")
}

func LoadFrom(path string) (*Config, error) {
	cfg := &Config{path: path}
	path = cfg.Path()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, err
	}

	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c *Config) Save() error {
	dir := c.ConfigDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(c.Path(), data, 0600)
}

func (c *Config) IsLoggedIn() bool {
	cred, err := c.loadCredentials()
	if err != nil {
		return false
	}
	return cred.AccessToken != "" && (cred.RefreshToken != "" || cred.RefreshToken == "api_key")
}

func (c *Config) GetAccessToken() string {
	cred, _ := c.loadCredentials()
	if cred != nil {
		return cred.AccessToken
	}
	return ""
}

func (c *Config) SetCredentials(access, refresh string) error {
	cred := &Credentials{
		AccessToken:  access,
		RefreshToken: refresh,
	}
	return c.saveCredentials(cred)
}

func DefaultConfig() *Config {
	return DefaultConfigFrom("")
}

func DefaultConfigFrom(path string) *Config {
	return &Config{
		Region: "global",
		path:   path,
	}
}

func (c *Config) APIEndpoint() string {
	if c.Region == "china" {
		return "https://api.minimaxi.com"
	}
	return "https://api.minimax.io"
}
