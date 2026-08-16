package suno

import "testing"

func TestDeriveTrackType(t *testing.T) {
	cases := []struct {
		meta, entity, want string
	}{
		{"cover", "", "cover"},
		{"", "extend", "extend"},
		{"gen", "", "full_song"},
		{"generate", "", "full_song"},
		{"full_song", "", "full_song"},
		{"Cover", "", "cover"},
		{"", "", ""},
		{"unknown", "weird", ""},
		{"extend", "cover", "extend"}, // meta wins
	}
	for _, c := range cases {
		got := deriveTrackType(c.meta, c.entity)
		if got != c.want {
			t.Errorf("deriveTrackType(%q,%q)=%q want %q", c.meta, c.entity, got, c.want)
		}
	}
}

func TestConvertTrackMetrics(t *testing.T) {
	api := apiTrack{
		ID:          "abc",
		Title:       "Song",
		PlayCount:   7,
		UpvoteCount: 3,
		IsLiked:     true,
		EntityType:  "song",
		ModelName:   "v4",
		Metadata:    TrackMetadata{Type: "cover", Prompt: "p", Tags: "rock"},
	}
	got := convertTrack(api)
	if got.PlayCount != 7 || got.UpvoteCount != 3 || !got.IsLiked {
		t.Errorf("counts: play=%d upvote=%d liked=%v", got.PlayCount, got.UpvoteCount, got.IsLiked)
	}
	if got.TrackType != "cover" {
		t.Errorf("TrackType=%q want cover", got.TrackType)
	}
	if got.ModelName != "v4" {
		t.Errorf("ModelName=%q", got.ModelName)
	}
	if got.Prompt != "p" {
		t.Errorf("Prompt=%q", got.Prompt)
	}
}
