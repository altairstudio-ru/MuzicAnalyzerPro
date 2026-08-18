package library

import (
	"testing"

	"github.com/altairstudio-ru/MuzicAnalyzerPro/pkg/models"
)

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

func TestSyncLimitResolution(t *testing.T) {
	cases := []struct {
		name string
		opts *models.SyncOptions
		want int
	}{
		{"nil opts means no limit", nil, 0},
		{"limit set", &models.SyncOptions{Limit: 10}, 10},
		{"newest set", &models.SyncOptions{Newest: 7}, 7},
		{"newest beats limit", &models.SyncOptions{Limit: 10, Newest: 7}, 7},
		{"both zero means no limit", &models.SyncOptions{}, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := syncLimit(tc.opts); got != tc.want {
				t.Errorf("syncLimit(%+v) = %d, want %d", tc.opts, got, tc.want)
			}
		})
	}
}
