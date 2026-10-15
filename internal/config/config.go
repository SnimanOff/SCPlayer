package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type Config struct {
	DiscordID string `json:"discord_client_id"`
	MPVPath   string `json:"mpv_path,omitempty"`
}

func Load() (Config, error) {
	path := os.Getenv("SC_PLAYER_CONFIG")
	explicit := path != ""
	if !explicit {
		executable, err := os.Executable()
		if err != nil {
			return Config{}, err
		}
		path = filepath.Join(filepath.Dir(executable), "sc-player.json")
	}
	return load(path, explicit)
}

func load(path string, required bool) (Config, error) {
	var cfg Config
	data, err := os.ReadFile(path)
	if err != nil && (required || !os.IsNotExist(err)) {
		return cfg, fmt.Errorf("конфигурация: %w", err)
	}
	if err == nil {
		if err := json.Unmarshal(data, &cfg); err != nil {
			return cfg, fmt.Errorf("конфигурация: %w", err)
		}
	}
	for name, target := range map[string]*string{
		"DISCORD_CLIENT_ID": &cfg.DiscordID, "MPV_PATH": &cfg.MPVPath,
	} {
		if value, ok := os.LookupEnv(name); ok {
			*target = value
		}
	}
	return cfg, nil
}
