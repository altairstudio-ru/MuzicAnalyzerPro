package db

import (
	"testing"

	"github.com/altairstudio-ru/MuzicAnalyzerPro/pkg/models"
)

func TestDefaultLabelsSeeded(t *testing.T) {
	d := newTestDB(t)
	defer d.Close()

	labels, err := ListLabels(d)
	if err != nil {
		t.Fatalf("ListLabels: %v", err)
	}
	found := map[string]bool{}
	for _, l := range labels {
		found[l.Name] = true
	}
	for _, want := range DefaultLabels {
		if !found[want.Name] {
			t.Errorf("default label %q missing", want.Name)
		}
	}

	// EnsureDefaultLabels is idempotent.
	if err := EnsureDefaultLabels(d); err != nil {
		t.Fatalf("EnsureDefaultLabels: %v", err)
	}
	labels, _ = ListLabels(d)
	if len(labels) != len(DefaultLabels) {
		t.Errorf("labels after reseed = %d, want %d", len(labels), len(DefaultLabels))
	}
}

func TestLabelLifecycle(t *testing.T) {
	d := newTestDB(t)
	defer d.Close()

	lbl := &models.Label{Name: "radio", Color: "#ffb454"}
	if err := CreateLabel(d, lbl); err != nil {
		t.Fatalf("CreateLabel: %v", err)
	}
	if lbl.ID == "" {
		t.Fatal("CreateLabel did not generate ID")
	}

	// Duplicate name rejected.
	if err := CreateLabel(d, &models.Label{Name: "radio"}); err == nil {
		t.Error("duplicate label name should error")
	}

	// Update.
	if err := UpdateLabel(d, lbl.ID, "radio-edit", "#6ee7a0"); err != nil {
		t.Fatalf("UpdateLabel: %v", err)
	}
	got, err := GetLabel(d, lbl.ID)
	if err != nil || got == nil {
		t.Fatalf("GetLabel: %v err=%v", got, err)
	}
	if got.Name != "radio-edit" || got.Color != "#6ee7a0" {
		t.Errorf("updated label = %+v", got)
	}

	// Assign to tracks.
	mustTrack(t, d, "t1", "One", true)
	mustTrack(t, d, "t2", "Two", true)
	if err := SetTrackLabels(d, "t1", []string{lbl.ID}); err != nil {
		t.Fatalf("SetTrackLabels: %v", err)
	}
	// Assigning a missing label rolls the whole set back (transaction).
	if err := SetTrackLabels(d, "t2", []string{lbl.ID, "not-a-label"}); err == nil {
		t.Fatal("assigning a missing label should error")
	}
	t2Labels, _ := GetTrackLabels(d, "t2")
	if len(t2Labels) != 0 {
		t.Fatalf("t2 labels after failed assign = %+v, want none (rolled back)", t2Labels)
	}
	if err := SetTrackLabels(d, "t2", []string{lbl.ID}); err != nil {
		t.Fatalf("SetTrackLabels t2: %v", err)
	}

	t1Labels, err := GetTrackLabels(d, "t1")
	if err != nil {
		t.Fatalf("GetTrackLabels: %v", err)
	}
	if len(t1Labels) != 1 || t1Labels[0].ID != lbl.ID {
		t.Errorf("t1 labels = %+v", t1Labels)
	}

	// Replacement: t1 gets a second label, radio stays.
	second := &models.Label{Name: "hit"}
	if err := CreateLabel(d, second); err != nil {
		t.Fatalf("CreateLabel second: %v", err)
	}
	if err := SetTrackLabels(d, "t1", []string{lbl.ID, second.ID}); err != nil {
		t.Fatalf("SetTrackLabels replace: %v", err)
	}
	t1Labels, _ = GetTrackLabels(d, "t1")
	if len(t1Labels) != 2 {
		t.Errorf("t1 labels after replace = %+v", t1Labels)
	}

	// ListTracks by label name.
	byLabel, err := ListTracksByLabel(d, "radio-edit", models.TrackFilter{})
	if err != nil {
		t.Fatalf("ListTracksByLabel: %v", err)
	}
	if gotIDs := ids(byLabel); len(gotIDs) != 2 {
		t.Errorf("tracks with label = %v, want 2", gotIDs)
	}

	// TrackCount in ListLabels.
	all, _ := ListLabels(d)
	var radio models.Label
	for _, l := range all {
		if l.ID == lbl.ID {
			radio = l
		}
	}
	if radio.TrackCount != 2 {
		t.Errorf("label track count = %d, want 2", radio.TrackCount)
	}

	// Delete label cascades assignments.
	if err := DeleteLabel(d, lbl.ID); err != nil {
		t.Fatalf("DeleteLabel: %v", err)
	}
	t1Labels, _ = GetTrackLabels(d, "t1")
	if len(t1Labels) != 1 || t1Labels[0].ID != second.ID {
		t.Errorf("t1 labels after delete = %+v", t1Labels)
	}
}
