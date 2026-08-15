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

func TestSetAuthTokenBumpsGeneration(t *testing.T) {
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

	before := m.tokenGeneration()
	m.SetAuthToken("fresh-token")
	if m.tokenGeneration() != before+1 {
		t.Errorf("tokenGeneration() = %d after SetAuthToken, want %d", m.tokenGeneration(), before+1)
	}
}

func TestSyncStatusWaitingAuthPassthrough(t *testing.T) {
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

	m.beginSync()
	m.updateSync(func(s *SyncStatus) {
		s.WaitingAuth = true
	})
	st := m.GetSyncStatus()
	if !st.WaitingAuth {
		t.Errorf("WaitingAuth = false, want true")
	}
	m.finishSync()

	m.beginSync()
	if st := m.GetSyncStatus(); st.WaitingAuth {
		t.Errorf("WaitingAuth not reset by beginSync")
	}
	m.finishSync()
}
