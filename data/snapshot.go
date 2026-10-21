package data

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	snapshotFileName  = "last_problems.json"
	snapshotTmpSuffix = ".tmp"
	// maxSnapshotProblems ограничивает размер файла снапшота.
	maxSnapshotProblems = 500
	// snapshotMaxAge — старше этого снапшот не показываем даже как кеш.
	snapshotMaxAge = 24 * time.Hour
)

// ProblemsSnapshot — последний удачный ответ API. Нужен, чтобы при запуске
// приложение сразу показало данные, а не пустой экран «Ожидание данных...».
//
// Секретов в файле нет: вместо токена хранится необратимый отпечаток
// Config (см. SnapshotKey), поэтому снапшот нельзя использовать для доступа к
// серверу и он не показывается, если настройки сменились.
type ProblemsSnapshot struct {
	Key      string    `json:"key"`
	SavedAt  int64     `json:"saved_at"`
	Problems []Problem `json:"problems"`
}

// SnapshotKey возвращает необратимый отпечаток конфигурации: смена сервера или
// токена делает старый снапшот недействительным.
func SnapshotKey(cfg Config) string {
	sum := sha256.Sum256([]byte(NormalizeURL(cfg.URL) + "\x00" + cfg.Token + "\x00" + cfg.User))
	return hex.EncodeToString(sum[:8])
}

// SnapshotStore хранит снапшот рядом с настройками.
type SnapshotStore struct {
	mu          sync.Mutex
	dir         string
	skipDirSync bool
}

// NewSnapshotStore создаёт хранилище снапшота рядом с настройками.
func NewSnapshotStore() *SnapshotStore {
	return &SnapshotStore{dir: DefaultSettingsDir()}
}

// NewSnapshotStoreIn создаёт хранилище снапшота в указанном каталоге.
func NewSnapshotStoreIn(dir string) *SnapshotStore {
	return &SnapshotStore{dir: dir}
}

func (s *SnapshotStore) path() string { return filepath.Join(s.dir, snapshotFileName) }

// Load возвращает снапшот, если он относится к текущей конфигурации и не устарел.
func (s *SnapshotStore) Load(key string, now time.Time) ([]Problem, time.Time, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	raw, err := readStateFile(s.path())
	if err != nil {
		return nil, time.Time{}, false
	}

	var snap ProblemsSnapshot
	if err := json.Unmarshal(raw, &snap); err != nil {
		return nil, time.Time{}, false
	}
	if snap.Key != key || len(snap.Problems) == 0 {
		return nil, time.Time{}, false
	}

	savedAt := time.Unix(snap.SavedAt, 0)
	if now.Sub(savedAt) > snapshotMaxAge {
		return nil, time.Time{}, false
	}
	return snap.Problems, savedAt, true
}

// Save атомарно записывает снапшот. Ошибки здесь не критичны для работы.
func (s *SnapshotStore) Save(key string, problems []Problem) error {
	if len(problems) == 0 {
		s.Clear()
		return nil
	}
	if len(problems) > maxSnapshotProblems {
		problems = problems[:maxSnapshotProblems]
	}

	payload, err := json.Marshal(ProblemsSnapshot{
		Key:      key,
		SavedAt:  time.Now().Unix(),
		Problems: problems,
	})
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	return writeAtomic(s.path(), payload, 0o600, !s.skipDirSync)
}

// Clear удаляет снапшот (например, при сбросе настроек).
func (s *SnapshotStore) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()

	_ = os.Remove(s.path())
	_ = os.Remove(s.path() + snapshotTmpSuffix)
}
