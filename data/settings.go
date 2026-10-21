package data

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Имена файлов настроек. Файл лежит в приватном каталоге приложения, поэтому
// переживает перезапуск, обновление APK и очистку кеша.
const (
	settingsFileName = "settings.json"
	settingsTmpName  = "settings.json.tmp"
	settingsBakName  = "settings.json.bak"
)

// Имена файлов настроек, которые может вернуть LoadWithSource.
const (
	SettingsMainFile   = settingsFileName
	SettingsBackupFile = settingsBakName
)

// DefaultRefreshSeconds / DefaultLimit — значения по умолчанию для интервала
// автообновления и лимита отображаемых проблем.
const (
	DefaultRefreshSeconds = 60
	DefaultLimit          = 200
)

// ErrNoSettings возвращается, когда ни основной файл, ни резервная копия не
// содержат пригодных настроек.
var ErrNoSettings = errors.New("settings: нет сохранённых настроек")

// errEmptySettings — файл прочитался, но в нём нет ни адреса сервера, ни
// учётных данных. Такой файл считается испорченным: пробуем .bak.
var errEmptySettings = errors.New("settings: файл не содержит данных")

// Ошибки валидации настроек.
var (
	ErrURLRequired         = errors.New("settings: не задан адрес сервера")
	ErrURLInvalid          = errors.New("settings: некорректный адрес сервера")
	ErrCredentialsRequired = errors.New("settings: нужен токен или логин с паролем")
)

// Settings — всё, что приложение помнит между запусками.
//
// Токен и пароль хранятся в типе Secret: он маскируется при печати, поэтому
// случайный лог или сообщение об ошибке не сольют учётные данные.
type Settings struct {
	URL            string `json:"url,omitempty"`
	Token          Secret `json:"token,omitempty"`
	User           string `json:"user,omitempty"`
	Password       Secret `json:"password,omitempty"`
	SelfSigned     bool   `json:"self_signed,omitempty"`
	RefreshSeconds int    `json:"refresh_seconds,omitempty"`
	Limit          int    `json:"limit,omitempty"`
	Theme          string `json:"theme,omitempty"`
	Lang           string `json:"lang,omitempty"`
	UpdatedAt      int64  `json:"updated_at,omitempty"`
}

// DefaultSettings — значения, с которыми приложение запускается впервые.
func DefaultSettings() Settings {
	return Settings{
		RefreshSeconds: DefaultRefreshSeconds,
		Limit:          DefaultLimit,
		Theme:          "dark",
		Lang:           "ru",
	}
}

// Normalized приводит настройки к предсказуемому виду: обрезает пробелы,
// нормализует URL и подставляет значения по умолчанию.
func (s Settings) Normalized() Settings {
	s.URL = NormalizeURL(s.URL)

	// Учётные данные, случайно вставленные в URL (https://user:pass@host),
	// переносим в поля и вырезаем из адреса: иначе они попадут в логи,
	// в историю и в файлы настроек в открытом виде.
	if u, err := url.Parse(s.URL); err == nil && u.User != nil {
		if pass, ok := u.User.Password(); ok && !s.Password.IsSet() {
			s.Password = Secret(pass)
		}
		if name := u.User.Username(); name != "" && s.User == "" {
			s.User = name
		}
		u.User = nil
		s.URL = u.String()
	}

	s.Token = Secret(strings.TrimSpace(string(s.Token)))
	s.User = strings.TrimSpace(s.User)
	s.Password = Secret(strings.TrimSpace(string(s.Password)))

	if s.RefreshSeconds <= 0 {
		s.RefreshSeconds = DefaultRefreshSeconds
	}
	if s.RefreshSeconds < 10 {
		s.RefreshSeconds = 10
	}
	if s.Limit <= 0 {
		s.Limit = DefaultLimit
	}
	switch s.Theme {
	case "light", "dark":
	default:
		s.Theme = "dark"
	}
	switch s.Lang {
	case "ru", "en":
	default:
		s.Lang = "ru"
	}
	return s
}

// HasCredentials сообщает, есть ли чем авторизоваться: токен или пара
// логин/пароль (Basic Auth).
func (s Settings) HasCredentials() bool {
	return s.Token.IsSet() || (s.User != "" && s.Password.IsSet())
}

// ValidateURL проверяет адрес сервера (после NormalizeURL).
func ValidateURL(raw string) error {
	if strings.TrimSpace(raw) == "" {
		return ErrURLRequired
	}
	u, err := url.Parse(NormalizeURL(raw))
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return ErrURLInvalid
	}
	return nil
}

// Validate проверяет настройки перед сохранением.
func (s Settings) Validate() error {
	if err := ValidateURL(s.URL); err != nil {
		return err
	}
	if !s.HasCredentials() {
		return ErrCredentialsRequired
	}
	return nil
}

// Interval возвращает интервал автообновления.
func (s Settings) Interval() time.Duration {
	if s.RefreshSeconds <= 0 {
		return DefaultRefreshSeconds * time.Second
	}
	return time.Duration(s.RefreshSeconds) * time.Second
}

// ClientConfig собирает конфигурацию API-клиента из настроек.
func (s Settings) ClientConfig() Config {
	st := s.Normalized()
	cfg := NewConfig()
	cfg.URL = st.URL
	cfg.Token = st.Token.Value()
	cfg.User = st.User
	cfg.Password = st.Password.Value()
	cfg.SelfSigned = st.SelfSigned
	cfg.Limit = st.Limit
	return cfg
}

// Store — надёжное хранилище настроек.
//
// Запись атомарна (tmp + fsync + rename + fsync каталога), а предыдущая версия
// файла сохраняется как settings.json.bak. Это важно на Android: система может
// убить процесс или телефон может разрядиться в любой момент, а Fyne пишет свой
// preferences.json напрямую с усечением файла — после такого сбоя файл остаётся
// пустым и настройки теряются.
type Store struct {
	mu  sync.Mutex
	dir string

	// syncDir отключается в тестах (fsync каталога не нужен и не всегда доступен).
	skipDirSync bool
}

// NewStore создаёт хранилище в приватном каталоге приложения.
func NewStore() *Store {
	return &Store{dir: DefaultSettingsDir()}
}

// NewStoreIn создаёт хранилище в указанном каталоге (используется в тестах).
func NewStoreIn(dir string) *Store {
	return &Store{dir: dir}
}

// Dir возвращает каталог, в котором лежат файлы настроек.
func (s *Store) Dir() string { return s.dir }

func (s *Store) path() string       { return filepath.Join(s.dir, settingsFileName) }
func (s *Store) backupPath() string { return filepath.Join(s.dir, settingsBakName) }

// Load читает основной файл, при ошибке — резервную копию.
func (s *Store) Load() (Settings, error) {
	st, _, err := s.LoadWithSource()
	return st, err
}

// LoadWithSource то же, что Load, но дополнительно сообщает, откуда пришли
// настройки ("" — файлов нет, settingsFileName — основной,
// settingsBakName — резервная копия). Нужно для диагностики на устройстве.
func (s *Store) LoadWithSource() (Settings, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var errs []error
	for _, name := range []string{settingsFileName, settingsBakName} {
		st, err := readSettingsFile(filepath.Join(s.dir, name))
		if err != nil {
			errs = append(errs, err)
			continue
		}
		return st, name, nil
	}
	return Settings{}, "", fmt.Errorf("%w: %w", ErrNoSettings, errors.Join(errs...))
}

// Save атомарно сохраняет настройки.
func (s *Store) Save(st Settings) error {
	st = st.Normalized()
	st.UpdatedAt = time.Now().Unix()

	payload, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	payload = append(payload, '\n')

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return fmt.Errorf("settings: %w", err)
	}

	// Сохраняем предыдущую версию как резервную, но только если она читается.
	if prev, err := readSettingsFileRaw(s.path()); err == nil {
		if _, perr := decodeSettings(prev); perr == nil {
			// не критично: при ошибке просто не будет резервной копии
			_ = writeFileSynced(s.backupPath(), prev, 0o600)
		}
	}

	if err := writeAtomic(s.path(), payload, 0o600, !s.skipDirSync); err != nil {
		return fmt.Errorf("settings: %w", err)
	}
	return nil
}

// Clear удаляет сохранённые настройки вместе с резервной копией.
//
// Используется кнопкой сброса: после неё на устройстве не остаётся токена.
func (s *Store) Clear() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var errs []error
	for _, name := range []string{settingsFileName, settingsBakName, settingsTmpName} {
		path := filepath.Join(s.dir, name)
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			errs = append(errs, err)
		}
	}
	if !s.skipDirSync {
		syncDir(s.dir)
	}
	return errors.Join(errs...)
}

func readSettingsFileRaw(path string) ([]byte, error) {
	return readStateFile(path)
}

func readSettingsFile(path string) (Settings, error) {
	raw, err := readSettingsFileRaw(path)
	if err != nil {
		return Settings{}, err
	}
	st, err := decodeSettings(raw)
	if err != nil {
		return Settings{}, fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	if st.URL == "" && !st.HasCredentials() {
		return Settings{}, fmt.Errorf("%s: %w", filepath.Base(path), errEmptySettings)
	}
	return st, nil
}

func decodeSettings(raw []byte) (Settings, error) {
	if len(raw) == 0 {
		return Settings{}, errEmptySettings
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return Settings{}, errEmptySettings
	}
	var st Settings
	if err := json.Unmarshal(raw, &st); err != nil {
		return Settings{}, err
	}
	return st.Normalized(), nil
}

// DefaultSettingsDir возвращает каталог настроек.
//
// Android: FILESDIR — приватный каталог приложения (getFilesDir()), он
// переживает перезапуски и обновления. На десктопе — конфиг-каталог ОС.
func DefaultSettingsDir() string {
	if dir := os.Getenv("FILESDIR"); dir != "" {
		// тот же каталог, куда Fyne кладёт свои preferences.json
		return filepath.Join(dir, "fyne")
	}
	if dir, err := os.UserConfigDir(); err == nil && dir != "" {
		return filepath.Join(dir, "zabbix-monitor")
	}
	if dir, err := os.UserHomeDir(); err == nil && dir != "" {
		return filepath.Join(dir, ".zabbix-monitor")
	}
	return filepath.Join(os.TempDir(), "zabbix-monitor")
}
