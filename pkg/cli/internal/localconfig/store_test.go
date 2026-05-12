package localconfig

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/adrg/xdg"
)

func TestNormalizeAppURL(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    string
		wantErr bool
	}{
		{
			name: "https",
			raw:  "https://obot.example.com",
			want: "https://obot.example.com",
		},
		{
			name: "http",
			raw:  "http://localhost:8080",
			want: "http://localhost:8080",
		},
		{
			name: "trim whitespace and slash",
			raw:  "  https://obot.example.com/  ",
			want: "https://obot.example.com",
		},
		{
			name: "trim multiple trailing slashes",
			raw:  "https://obot.example.com/path///",
			want: "https://obot.example.com/path",
		},
		{
			name: "accept API base URL",
			raw:  "https://obot.example.com/api",
			want: "https://obot.example.com",
		},
		{
			name: "accept nested API base URL",
			raw:  "https://obot.example.com/obot/api/",
			want: "https://obot.example.com/obot",
		},
		{
			name:    "empty",
			raw:     " ",
			wantErr: true,
		},
		{
			name:    "unsupported scheme",
			raw:     "ftp://obot.example.com",
			wantErr: true,
		},
		{
			name:    "missing scheme",
			raw:     "obot.example.com",
			wantErr: true,
		},
		{
			name:    "missing host",
			raw:     "https:///obot",
			wantErr: true,
		},
		{
			name:    "userinfo",
			raw:     "https://user:pass@obot.example.com",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeAppURL(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

func TestAPIBaseURL(t *testing.T) {
	got := APIBaseURL("https://obot.example.com")
	if got != "https://obot.example.com/api" {
		t.Fatalf("expected API URL, got %q", got)
	}

	got = APIBaseURL("https://obot.example.com/")
	if got != "https://obot.example.com/api" {
		t.Fatalf("expected API URL from trailing slash input, got %q", got)
	}

	appURL, err := NormalizeAppURL("https://obot.example.com/api")
	if err != nil {
		t.Fatal(err)
	}
	got = APIBaseURL(appURL)
	if got != "https://obot.example.com/api" {
		t.Fatalf("expected API URL from API base URL input, got %q", got)
	}
}

func TestLoadSaveConfig(t *testing.T) {
	configHome := useTestXDGConfigHome(t)

	if err := Save(Config{DefaultURL: " https://obot.example.com/ "}); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DefaultURL != "https://obot.example.com" {
		t.Fatalf("expected normalized default URL, got %q", cfg.DefaultURL)
	}

	data, err := os.ReadFile(filepath.Join(configHome, "obot", "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "{\n  \"defaultURL\": \"https://obot.example.com\"\n}\n" {
		t.Fatalf("unexpected config file:\n%s", string(data))
	}
}

func TestLoadMissingConfig(t *testing.T) {
	useTestXDGConfigHome(t)

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg != (Config{}) {
		t.Fatalf("expected empty config, got %#v", cfg)
	}
}

func TestActiveAppURL(t *testing.T) {
	useTestXDGConfigHome(t)

	if err := Save(Config{DefaultURL: "https://stored.example.com"}); err != nil {
		t.Fatal(err)
	}

	got, err := ActiveAppURL(" https://explicit.example.com/ ")
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://explicit.example.com" {
		t.Fatalf("expected explicit URL, got %q", got)
	}

	got, err = ActiveAppURL("")
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://stored.example.com" {
		t.Fatalf("expected stored URL, got %q", got)
	}
}

func TestActiveAppURLNoConfig(t *testing.T) {
	useTestXDGConfigHome(t)

	if _, err := ActiveAppURL(""); err == nil {
		t.Fatalf("expected error")
	}
}

func useTestXDGConfigHome(t *testing.T) string {
	t.Helper()

	configHome := filepath.Join(t.TempDir(), "config")
	oldConfigHome, hadConfigHome := os.LookupEnv("XDG_CONFIG_HOME")
	if err := os.Setenv("XDG_CONFIG_HOME", configHome); err != nil {
		t.Fatal(err)
	}
	xdg.Reload()
	t.Cleanup(func() {
		if hadConfigHome {
			_ = os.Setenv("XDG_CONFIG_HOME", oldConfigHome)
		} else {
			_ = os.Unsetenv("XDG_CONFIG_HOME")
		}
		xdg.Reload()
	})
	return configHome
}
