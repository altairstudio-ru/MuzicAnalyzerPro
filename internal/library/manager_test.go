package library

import "testing"

func TestStopSyncSetsCanceled(t *testing.T) {
	m, err := NewManager(&Config{
		Suno: SunoConfig{
			AuthToken: "test-token",
			BasePath:  t.TempDir(),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()

	if m.isCanceled() {
		t.Errorf("isCanceled() = true before any sync started")
	}

	if !m.beginSync() {
		t.Fatal("beginSync failed on fresh manager")
	}
	if m.isCanceled() {
		t.Errorf("isCanceled() = true after beginSync")
	}

	m.StopSync()
	if !m.isCanceled() {
		t.Errorf("isCanceled() = false after StopSync")
	}

	m.finishSync()
	st := m.GetSyncStatus()
	if !st.Stopped {
		t.Errorf("Stopped = false after StopSync")
	}
}

func TestBeginSyncRejectsDoubleStart(t *testing.T) {
	m, err := NewManager(&Config{
		Suno: SunoConfig{
			AuthToken: "test-token",
			BasePath:  t.TempDir(),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()

	if !m.beginSync() {
		t.Fatal("first beginSync failed")
	}
	if m.beginSync() {
		t.Errorf("second beginSync succeeded while one running")
	}
	m.finishSync()
}
