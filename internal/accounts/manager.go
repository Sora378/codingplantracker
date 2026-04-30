package accounts

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/99designs/keyring"
	"github.com/Sora378/codingplantracker/internal/api"
	"github.com/Sora378/codingplantracker/internal/codex"
	"github.com/Sora378/codingplantracker/internal/config"
	"github.com/Sora378/codingplantracker/internal/models"
)

const (
	ProviderCodex   = "codex"
	ProviderMiniMax = "minimax"

	keyringService = "coplanage"
	legacyKey      = "credentials"
)

var errProfileNotFound = errors.New("profile not found")

type Profile = config.Profile

type Status struct {
	Connected bool
	Label     string
	Error     string
}

type Manager struct {
	cfg *config.Config
}

func NewManager(cfg *config.Config) *Manager {
	m := &Manager{cfg: cfg}
	m.ensureDefaults()
	return m
}

func (m *Manager) Config() *config.Config {
	return m.cfg
}

func (m *Manager) Profiles() []Profile {
	profiles := make([]Profile, len(m.cfg.Profiles))
	copy(profiles, m.cfg.Profiles)
	return profiles
}

func (m *Manager) ActiveProfileID() string {
	return m.cfg.ActiveProfileID
}

func (m *Manager) ActiveProfile() Profile {
	if profile, ok := m.Find(m.cfg.ActiveProfileID); ok {
		return profile
	}
	m.ensureDefaults()
	profile, _ := m.Find(m.cfg.ActiveProfileID)
	return profile
}

func (m *Manager) Find(id string) (Profile, bool) {
	for _, profile := range m.cfg.Profiles {
		if profile.ID == id {
			return profile, true
		}
	}
	return Profile{}, false
}

func (m *Manager) Switch(id string) error {
	for i := range m.cfg.Profiles {
		if m.cfg.Profiles[i].ID == id {
			now := time.Now().Format(time.RFC3339)
			m.cfg.ActiveProfileID = id
			m.cfg.Profiles[i].LastUsedAt = now
			return m.cfg.Save()
		}
	}
	return errProfileNotFound
}

func (m *Manager) AddCodex(name, codexHome string) (Profile, error) {
	if strings.TrimSpace(name) == "" {
		name = "Codex"
	}
	profile := Profile{
		ID:        uniqueProfileID(m.cfg.Profiles, "codex-"+slug(name)),
		Name:      strings.TrimSpace(name),
		Provider:  ProviderCodex,
		CodexHome: strings.TrimSpace(codexHome),
		CreatedAt: time.Now().Format(time.RFC3339),
	}
	m.cfg.Profiles = append(m.cfg.Profiles, profile)
	if m.cfg.ActiveProfileID == "" {
		m.cfg.ActiveProfileID = profile.ID
	}
	return profile, m.cfg.Save()
}

func (m *Manager) AddMiniMax(ctx context.Context, name, region, apiKey string, validate bool) (Profile, error) {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return Profile{}, errors.New("MiniMax API key cannot be empty")
	}
	if region != "china" {
		region = "global"
	}
	if strings.TrimSpace(name) == "" {
		name = "MiniMax"
	}
	profile := Profile{
		ID:        uniqueProfileID(m.cfg.Profiles, "minimax-"+slug(name)),
		Name:      strings.TrimSpace(name),
		Provider:  ProviderMiniMax,
		Region:    region,
		CreatedAt: time.Now().Format(time.RFC3339),
	}
	if validate {
		if err := validateMiniMax(ctx, profile, apiKey); err != nil {
			return Profile{}, err
		}
	}
	if err := saveSecret(profile.ID, apiKey); err != nil {
		return Profile{}, err
	}
	m.cfg.Profiles = append(m.cfg.Profiles, profile)
	if m.cfg.ActiveProfileID == "" {
		m.cfg.ActiveProfileID = profile.ID
	}
	return profile, m.cfg.Save()
}

func (m *Manager) Logout(id string) error {
	profile, ok := m.Find(id)
	if !ok {
		return errProfileNotFound
	}
	switch profile.Provider {
	case ProviderMiniMax:
		return clearSecret(id)
	case ProviderCodex:
		bin, err := codex.BinaryPath()
		if err != nil {
			return err
		}
		cmd := exec.Command(bin, "logout")
		cmd.Env = codexEnv(profile)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("%w: %s", err, cleanOutput(out))
		}
	}
	return nil
}

func (m *Manager) Remove(id string) error {
	index := -1
	for i, profile := range m.cfg.Profiles {
		if profile.ID == id {
			index = i
			break
		}
	}
	if index < 0 {
		return errProfileNotFound
	}
	_ = clearSecret(id)
	m.cfg.Profiles = append(m.cfg.Profiles[:index], m.cfg.Profiles[index+1:]...)
	if m.cfg.ActiveProfileID == id {
		m.cfg.ActiveProfileID = ""
		if len(m.cfg.Profiles) > 0 {
			m.cfg.ActiveProfileID = m.cfg.Profiles[0].ID
		}
	}
	m.ensureDefaults()
	return m.cfg.Save()
}

func (m *Manager) Status(ctx context.Context, profile Profile) Status {
	switch profile.Provider {
	case ProviderMiniMax:
		secret, err := loadSecret(profile.ID)
		if err != nil || secret == "" {
			return Status{Label: "Logged out", Error: "MiniMax API key missing"}
		}
		return Status{Connected: true, Label: "Connected"}
	case ProviderCodex:
		bin, err := codex.BinaryPath()
		if err != nil {
			return Status{Label: "CLI not found", Error: err.Error()}
		}
		cmd := exec.CommandContext(ctx, bin, "login", "status")
		cmd.Env = codexEnv(profile)
		if out, err := cmd.CombinedOutput(); err != nil {
			return Status{Label: "Disconnected", Error: cleanOutput(out)}
		}
		return Status{Connected: true, Label: "Connected"}
	default:
		return Status{Label: "Unknown provider", Error: profile.Provider}
	}
}

func (m *Manager) ReadCodexUsage(ctx context.Context, profile Profile) (*codex.Usage, error) {
	return codex.ReadUsageWithEnv(ctx, codexEnv(profile))
}

func (m *Manager) ReadMiniMaxUsage(ctx context.Context, profile Profile) (*models.CurrentUsage, error) {
	secret, err := loadSecret(profile.ID)
	if err != nil {
		return nil, err
	}
	cfg := config.DefaultConfig()
	cfg.Region = profile.Region
	client := api.NewClient(cfg)
	client.SetToken(secret)
	return client.GetCurrentUsage(ctx, cfg)
}

func (m *Manager) ensureDefaults() {
	if len(m.cfg.Profiles) == 0 {
		m.cfg.Profiles = append(m.cfg.Profiles, Profile{
			ID:        "codex-default",
			Name:      "Codex",
			Provider:  ProviderCodex,
			CreatedAt: time.Now().Format(time.RFC3339),
		})
		if m.cfg.ActiveProfileID == "" {
			m.cfg.ActiveProfileID = "codex-default"
		}
		return
	}
	if m.cfg.ActiveProfileID == "" {
		m.cfg.ActiveProfileID = m.cfg.Profiles[0].ID
	}
}

func MigrateLegacyMiniMax(cfg *config.Config) error {
	if hasMiniMaxProfile(cfg.Profiles) {
		return nil
	}
	secret, err := loadLegacySecret()
	if err != nil || secret == "" {
		return nil
	}
	profile := Profile{
		ID:        "minimax-default",
		Name:      "MiniMax",
		Provider:  ProviderMiniMax,
		Region:    cfg.Region,
		CreatedAt: time.Now().Format(time.RFC3339),
	}
	if profile.Region == "" {
		profile.Region = "global"
	}
	if err := saveSecret(profile.ID, secret); err != nil {
		return err
	}
	cfg.Profiles = append(cfg.Profiles, profile)
	if cfg.ActiveProfileID == "" {
		cfg.ActiveProfileID = profile.ID
	}
	return cfg.Save()
}

func hasMiniMaxProfile(profiles []Profile) bool {
	for _, profile := range profiles {
		if profile.Provider == ProviderMiniMax {
			return true
		}
	}
	return false
}

func validateMiniMax(ctx context.Context, profile Profile, apiKey string) error {
	cfg := config.DefaultConfig()
	cfg.Region = profile.Region
	client := api.NewClient(cfg)
	client.SetToken(apiKey)
	_, err := client.GetCurrentUsage(ctx, cfg)
	return err
}

func codexEnv(profile Profile) []string {
	if strings.TrimSpace(profile.CodexHome) == "" {
		return nil
	}
	return append(os.Environ(), "CODEX_HOME="+strings.TrimSpace(profile.CodexHome))
}

func saveSecret(profileID, secret string) error {
	kr, err := openKeyring()
	if err != nil {
		return err
	}
	return kr.Set(keyring.Item{Key: profileSecretKey(profileID), Data: []byte(secret)})
}

func loadSecret(profileID string) (string, error) {
	kr, err := openKeyring()
	if err != nil {
		return "", err
	}
	item, err := kr.Get(profileSecretKey(profileID))
	if err != nil {
		if err == keyring.ErrKeyNotFound {
			return "", nil
		}
		return "", err
	}
	return string(item.Data), nil
}

func clearSecret(profileID string) error {
	kr, err := openKeyring()
	if err != nil {
		return err
	}
	if err := kr.Remove(profileSecretKey(profileID)); err != nil && err != keyring.ErrKeyNotFound {
		return err
	}
	return nil
}

func loadLegacySecret() (string, error) {
	kr, err := openKeyring()
	if err != nil {
		return "", err
	}
	item, err := kr.Get(legacyKey)
	if err != nil {
		if err == keyring.ErrKeyNotFound {
			return "", nil
		}
		return "", err
	}
	var cred struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(item.Data, &cred); err == nil && cred.AccessToken != "" {
		return cred.AccessToken, nil
	}
	return string(item.Data), nil
}

func openKeyring() (keyring.Keyring, error) {
	return keyring.Open(keyring.Config{ServiceName: keyringService})
}

func profileSecretKey(profileID string) string {
	return "profile:" + profileID
}

var slugPattern = regexp.MustCompile(`[^a-z0-9]+`)

func slug(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = slugPattern.ReplaceAllString(value, "-")
	value = strings.Trim(value, "-")
	if value == "" {
		return "account"
	}
	return value
}

func uniqueProfileID(profiles []Profile, base string) string {
	if base == "" {
		base = "account"
	}
	candidate := base
	for i := 2; profileIDExists(profiles, candidate); i++ {
		candidate = fmt.Sprintf("%s-%d", base, i)
	}
	return candidate
}

func profileIDExists(profiles []Profile, id string) bool {
	for _, profile := range profiles {
		if profile.ID == id {
			return true
		}
	}
	return false
}

func cleanOutput(out []byte) string {
	text := strings.Join(strings.Fields(string(out)), " ")
	if text == "" {
		return "command failed"
	}
	if len(text) > 100 {
		return text[:100]
	}
	return text
}
