package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigurationFileAndEnvironment(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"discord_client_id":"123","mpv_path":"/opt/mpv"}`), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MPV_PATH", "override")
	cfg, err := load(path, true)
	if err != nil || cfg.MPVPath != "override" || cfg.DiscordID != "123" {
		t.Fatalf("config: %+v %v", cfg, err)
	}
	t.Setenv("DISCORD_CLIENT_ID", "")
	cfg, err = load(path, true)
	if err != nil || cfg.DiscordID != "" {
		t.Fatal("empty environment override ignored")
	}
	if _, err := load(path+".missing", true); err == nil {
		t.Fatal("explicit missing config accepted")
	}
	if _, err := load(path+".missing", false); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("invalid JSON"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := load(path, true); err == nil {
		t.Fatal("invalid config accepted")
	}
}
