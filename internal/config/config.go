package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type APIConfig struct {
	Port          string
	MongoURI      string
	MongoDatabase string
	Tokens        string
	AdminToken    string
}

func LoadAPIConfig() (APIConfig, error) {
	cfg := APIConfig{
		Port:          getEnv("PORT", "8080"),
		MongoURI:      getEnv("MONGO_URI", "mongodb://localhost:27017"),
		MongoDatabase: getEnv("MONGO_DATABASE", "agent_memory"),
		Tokens:        os.Getenv("MEMORY_TOKENS"),
		AdminToken:    os.Getenv("ADMIN_TOKEN"),
	}
	if cfg.Tokens == "" {
		return cfg, fmt.Errorf("MEMORY_TOKENS env var required")
	}
	return cfg, nil
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

type WorkstationConfig struct {
	Workstation string   `yaml:"workstation"`
	DefaultOrg  string   `yaml:"default_org"`
	AllowedOrgs []string `yaml:"allowed_orgs"`
	APIURL      string   `yaml:"api_url"`
	TokenEnv    string   `yaml:"token_env"`
}

type RepoConfig struct {
	Org     string     `yaml:"org"`
	Project string     `yaml:"project,omitempty"`
	Repo    string     `yaml:"repo,omitempty"`
	Sync    SyncConfig `yaml:"sync,omitempty"`
}

type SyncConfig struct {
	OutputDir string   `yaml:"output_dir"`
	Files     []string `yaml:"files,omitempty"`
}

func LoadWorkstationConfig() (*WorkstationConfig, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("get home dir: %w", err)
	}
	path := filepath.Join(home, ".agent-memory", "config.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var cfg WorkstationConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse workstation config: %w", err)
	}
	return &cfg, nil
}

func LoadRepoConfig() (*RepoConfig, error) {
	data, err := os.ReadFile(".agent-memory.yaml")
	if err != nil {
		return nil, fmt.Errorf("read .agent-memory.yaml: %w (run `memory init` first)", err)
	}
	var cfg RepoConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse repo config: %w", err)
	}
	return &cfg, nil
}

func (w *WorkstationConfig) CanAccessOrg(org string) bool {
	for _, o := range w.AllowedOrgs {
		if o == org {
			return true
		}
	}
	return false
}

func (w *WorkstationConfig) Token() string {
	env := w.TokenEnv
	if env == "" {
		env = "MEMORY_TOKEN"
	}
	return strings.TrimSpace(os.Getenv(env))
}
