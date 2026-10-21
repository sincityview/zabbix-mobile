package data

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"
)

func sampleSettings() Settings {
	return Settings{
		URL:            "https://zabbix.example.com",
		Token:          Secret("secret-token"),
		RefreshSeconds: 30,
		Limit:          50,
		Theme:          "light",
		Lang:           "en",
	}
}

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s := NewStoreIn(t.TempDir())
	s.skipDirSync = true
	return s
}

func TestStore_LoadMissing(t *testing.T) {
	s := newTestStore(t)

	if _, err := s.Load(); !errors.Is(err, ErrNoSettings) {
		t.Fatalf("Load() error = %v, want ErrNoSettings", err)
	}
}

func TestStore_SaveLoadRoundTrip(t *testing.T) {
	s := newTestStore(t)
	want := sampleSettings()

	if err := s.Save(want); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	got, err := s.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	want.URL = NormalizeURL(want.URL) // сохраняется уже нормализованный URL
	if got.URL != want.URL || got.Token != want.Token || got.Limit != want.Limit ||
		got.RefreshSeconds != want.RefreshSeconds || got.Theme != want.Theme || got.Lang != want.Lang {
		t.Fatalf("Load() = %+v, want %+v", got, want)
	}
	if got.UpdatedAt == 0 {
		t.Error("UpdatedAt не заполнен")
	}
}

func TestStore_SaveCreatesDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "settings")
	s := NewStoreIn(dir)
	s.skipDirSync = true

	if err := s.Save(sampleSettings()); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, settingsFileName)); err != nil {
		t.Fatalf("файл настроек не создан: %v", err)
	}
}

func TestStore_NoTempFileLeftBehind(t *testing.T) {
	s := newTestStore(t)
	for i := 0; i < 3; i++ {
		if err := s.Save(sampleSettings()); err != nil {
			t.Fatalf("Save() error = %v", err)
		}
	}

	if _, err := os.Stat(filepath.Join(s.Dir(), settingsTmpName)); !os.IsNotExist(err) {
		t.Fatalf("временный файл остался после записи: %v", err)
	}
}

func TestStore_RecoversFromCorruptPrimary(t *testing.T) {
	s := newTestStore(t)
	if err := s.Save(sampleSettings()); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// вторая запись создаёт .bak с предыдущей версией
	updated := sampleSettings()
	updated.Token = Secret("second-token")
	if err := s.Save(updated); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// имитируем сбой: основной файл обрезан до нуля
	if err := os.WriteFile(filepath.Join(s.Dir(), settingsFileName), nil, 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := s.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.Token.Value() != "secret-token" {
		t.Fatalf("Token = %q, want значение из резервной копии", got.Token.Value())
	}
}

func TestStore_RecoversFromGarbagePrimary(t *testing.T) {
	s := newTestStore(t)
	if err := s.Save(sampleSettings()); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if err := s.Save(sampleSettings()); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	garbage := []byte(`{"url": "https://exa`)
	if err := os.WriteFile(filepath.Join(s.Dir(), settingsFileName), garbage, 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := s.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.URL == "" || got.Token == "" {
		t.Fatalf("Load() = %+v, want данные из резервной копии", got)
	}
}

func TestStore_DoesNotOverwriteGoodBackupWithGarbage(t *testing.T) {
	s := newTestStore(t)

	if err := s.Save(sampleSettings()); err != nil {
		t.Fatal(err)
	}
	if err := s.Save(sampleSettings()); err != nil {
		t.Fatal(err)
	}
	// портим основной файл и сохраняем новые настройки
	if err := os.WriteFile(filepath.Join(s.Dir(), settingsFileName), []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}

	updated := sampleSettings()
	updated.Token = Secret("third")
	if err := s.Save(updated); err != nil {
		t.Fatal(err)
	}

	// резервная копия должна остаться читаемой
	if _, err := readSettingsFile(filepath.Join(s.Dir(), settingsBakName)); err != nil {
		t.Fatalf("резервная копия испорчена: %v", err)
	}
	got, err := s.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.Token.Value() != "third" {
		t.Fatalf("Token = %q, want %q", got.Token.Value(), "third")
	}
}

func TestSettings_Normalized(t *testing.T) {
	st := Settings{URL: "  zabbix.local/zabbix/ ", RefreshSeconds: -5, Limit: 0}
	got := st.Normalized()

	if got.URL != "https://zabbix.local/zabbix/api_jsonrpc.php" {
		t.Errorf("URL = %q", got.URL)
	}
	if got.RefreshSeconds != DefaultRefreshSeconds {
		t.Errorf("RefreshSeconds = %d, want %d", got.RefreshSeconds, DefaultRefreshSeconds)
	}
	if got.Limit != DefaultLimit {
		t.Errorf("Limit = %d, want %d", got.Limit, DefaultLimit)
	}
	if got.Theme != "dark" || got.Lang != "ru" {
		t.Errorf("Theme/Lang = %q/%q", got.Theme, got.Lang)
	}
	if got.Password.Value() != "" {
		t.Errorf("Password = %q, want пусто", got.Password.Value())
	}
}

func TestSettings_Validate(t *testing.T) {
	tests := []struct {
		name string
		st   Settings
		want error
	}{
		{"без url", Settings{Token: Secret("t")}, ErrURLRequired},
		{"битый url", Settings{URL: "ftp://host", Token: Secret("t")}, ErrURLInvalid},
		{"без учётных данных", Settings{URL: "https://host"}, ErrCredentialsRequired},
		{"токен", sampleSettings(), nil},
		{"логин и пароль", Settings{URL: "https://host", User: "u", Password: Secret("p")}, nil},
		{"логин без пароля", Settings{URL: "https://host", User: "u"}, ErrCredentialsRequired},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.st.Validate()
			if !errors.Is(err, tt.want) {
				t.Fatalf("Validate() = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestSettings_Interval(t *testing.T) {
	st := Settings{}
	if got := st.Interval(); got != DefaultRefreshSeconds*time.Second {
		t.Errorf("Interval() = %v", got)
	}
	st.RefreshSeconds = 15
	if got := st.Interval(); got != 15*time.Second {
		t.Errorf("Interval() = %v", got)
	}
}

func TestSettings_ClientConfig(t *testing.T) {
	st := sampleSettings()
	cfg := st.ClientConfig()

	if cfg.URL != NormalizeURL(st.URL) {
		t.Errorf("URL = %q", cfg.URL)
	}
	if cfg.Limit != st.Limit {
		t.Errorf("Limit = %d", cfg.Limit)
	}
	if cfg.RetryCount == 0 || cfg.Timeout == 0 {
		t.Errorf("значения по умолчанию не применены: %+v", cfg)
	}
}

func TestDefaultSettingsDir_EnvOverride(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("FILESDIR", dir)

	if got := DefaultSettingsDir(); got != filepath.Join(dir, "fyne") {
		t.Errorf("DefaultSettingsDir() = %q", got)
	}
}

func TestStore_LoadWithSource(t *testing.T) {
	s := newTestStore(t)

	if _, source, err := s.LoadWithSource(); err == nil || source != "" {
		t.Fatalf("LoadWithSource() = %q, %v", source, err)
	}

	if err := s.Save(sampleSettings()); err != nil {
		t.Fatal(err)
	}
	if _, source, err := s.LoadWithSource(); err != nil || source != SettingsMainFile {
		t.Fatalf("source = %q, err = %v", source, err)
	}

	if err := s.Save(sampleSettings()); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.Dir(), settingsFileName), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	st, source, err := s.LoadWithSource()
	if err != nil {
		t.Fatalf("LoadWithSource() error = %v", err)
	}
	if source != SettingsBackupFile {
		t.Fatalf("source = %q, want %q", source, SettingsBackupFile)
	}
	if !st.Token.IsSet() {
		t.Error("настройки из резервной копии пусты")
	}
}

func TestStore_ConcurrentSaveLoad(t *testing.T) {
	s := newTestStore(t)
	if err := s.Save(sampleSettings()); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			st := sampleSettings()
			st.Token = Secret("token-" + strconv.Itoa(i))
			if err := s.Save(st); err != nil {
				t.Errorf("Save() error = %v", err)
			}
			if _, err := s.Load(); err != nil {
				t.Errorf("Load() error = %v", err)
			}
		}(i)
	}
	wg.Wait()

	if _, err := s.Load(); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
}
