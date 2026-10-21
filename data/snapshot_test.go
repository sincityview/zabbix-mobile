package data

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func testSnapshotKey() string {
	return SnapshotKey(Config{URL: "https://zabbix.example.com", Token: "token"})
}

func TestSnapshotKey_ChangesWithConfig(t *testing.T) {
	base := testSnapshotKey()

	otherToken := SnapshotKey(Config{URL: "https://zabbix.example.com", Token: "token2"})
	otherHost := SnapshotKey(Config{URL: "https://other.example.com", Token: "token"})

	if base == otherToken || base == otherHost {
		t.Fatal("отпечаток не меняется при смене сервера или токена")
	}
	if base != SnapshotKey(Config{URL: "https://zabbix.example.com/api_jsonrpc.php", Token: "token"}) {
		t.Error("нормализованный и ненормализованный URL должны давать один отпечаток")
	}
	if len(base) < 16 {
		t.Errorf("отпечаток слишком короткий: %q", base)
	}
	if base == "token" {
		t.Error("отпечаток не должен совпадать с секретом")
	}
}

func TestSnapshot_EmptyByDefault(t *testing.T) {
	s := NewSnapshotStoreIn(t.TempDir())
	if _, _, ok := s.Load(testSnapshotKey(), time.Now()); ok {
		t.Fatal("снапшот не должен загружаться, если его нет")
	}
}

func TestSnapshot_SaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s := NewSnapshotStoreIn(dir)
	s.skipDirSync = true

	want := []Problem{
		{EventID: "1", Name: "CPU high", Clock: "1700000000", Severity: "4", ObjectID: "100", HostName: "host-a"},
		{EventID: "2", Name: "Disk full", Clock: "1700000001", Severity: "5", ObjectID: "200", HostName: "host-b"},
	}

	if err := s.Save(testSnapshotKey(), want); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	got, savedAt, ok := s.Load(testSnapshotKey(), time.Now())
	if !ok {
		t.Fatal("снапшот не загрузился")
	}
	if !sameProblems(want, got) {
		t.Fatalf("Load() = %+v, want %+v", got, want)
	}
	if time.Since(savedAt) > time.Minute {
		t.Errorf("неверное время снапшота: %v", savedAt)
	}

	// В файле не должно быть ничего похожего на секрет.
	raw, err := os.ReadFile(filepath.Join(dir, snapshotFileName))
	if err != nil {
		t.Fatal(err)
	}
	for _, needle := range []string{"token", "Bearer", "password", "secret"} {
		if strings.Contains(string(raw), needle) {
			t.Fatalf("в снапшоте оказался секрет (%q): %s", needle, raw)
		}
	}
}

func TestSnapshot_KeyMismatchIgnored(t *testing.T) {
	s := NewSnapshotStoreIn(t.TempDir())
	s.skipDirSync = true

	if err := s.Save(testSnapshotKey(), []Problem{{EventID: "1"}}); err != nil {
		t.Fatal(err)
	}

	if _, _, ok := s.Load(SnapshotKey(Config{URL: "https://other", Token: "x"}), time.Now()); ok {
		t.Fatal("снапшот чужого сервера показывать нельзя")
	}
}

func TestSnapshot_Expires(t *testing.T) {
	s := NewSnapshotStoreIn(t.TempDir())
	s.skipDirSync = true

	if err := s.Save(testSnapshotKey(), []Problem{{EventID: "1"}}); err != nil {
		t.Fatal(err)
	}

	if _, _, ok := s.Load(testSnapshotKey(), time.Now().Add(snapshotMaxAge+time.Hour)); ok {
		t.Fatal("устаревший снапшот не должен возвращаться")
	}
}

func TestSnapshot_EmptyProblemsClearsFile(t *testing.T) {
	dir := t.TempDir()
	s := NewSnapshotStoreIn(dir)
	s.skipDirSync = true

	if err := s.Save(testSnapshotKey(), []Problem{{EventID: "1"}}); err != nil {
		t.Fatal(err)
	}
	if err := s.Save(testSnapshotKey(), nil); err != nil {
		t.Fatalf("Save(nil) error = %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, snapshotFileName)); !os.IsNotExist(err) {
		t.Fatalf("файл снапшота должен быть удалён: %v", err)
	}
}

func TestSnapshot_CorruptFileIgnored(t *testing.T) {
	dir := t.TempDir()
	s := NewSnapshotStoreIn(dir)
	s.skipDirSync = true

	if err := os.WriteFile(filepath.Join(dir, snapshotFileName), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := s.Load(testSnapshotKey(), time.Now()); ok {
		t.Fatal("битый снапшот не должен загружаться")
	}
}

func TestSnapshot_TruncatesHugeList(t *testing.T) {
	s := NewSnapshotStoreIn(t.TempDir())
	s.skipDirSync = true

	problems := make([]Problem, maxSnapshotProblems+50)
	for i := range problems {
		problems[i] = Problem{EventID: "e" + strconv.Itoa(i), Name: "problem"}
	}
	if err := s.Save(testSnapshotKey(), problems); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	got, _, ok := s.Load(testSnapshotKey(), time.Now())
	if !ok || len(got) != maxSnapshotProblems {
		t.Fatalf("got %d проблем, want %d", len(got), maxSnapshotProblems)
	}
}

func TestSnapshot_Clear(t *testing.T) {
	dir := t.TempDir()
	s := NewSnapshotStoreIn(dir)
	s.skipDirSync = true

	if err := s.Save(testSnapshotKey(), []Problem{{EventID: "1"}}); err != nil {
		t.Fatal(err)
	}
	s.Clear()

	if _, err := os.Stat(filepath.Join(dir, snapshotFileName)); !os.IsNotExist(err) {
		t.Fatalf("снапшот не удалён: %v", err)
	}
}

// sameProblems сравнивает два списка проблем по значению.
func sameProblems(a, b []Problem) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
