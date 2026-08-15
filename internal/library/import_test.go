package library

import "testing"

func TestSanitizeDirNameNoCollision(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"Поп", "Поп"},
		{"Рок", "Рок"},
		{"", ""},
		{"a/b:c", "a_b_c"},
		{"Поп Рок 2024", "Поп Рок 2024"},
	}
	seen := map[string]string{}
	for _, c := range cases {
		got := sanitizeDirName(c.in)
		if c.want != "" && got != c.want {
			t.Errorf("sanitizeDirName(%q) = %q, want %q", c.in, got, c.want)
		}
		if prev, dup := seen[got]; dup {
			t.Errorf("collision: sanitizeDirName(%q) = %q collides with %q", c.in, got, prev)
		}
		seen[got] = c.in
	}
}

func TestTrackIDFromFilename(t *testing.T) {
	cases := []struct {
		name string
		want string
	}{
		{"01 Ангел мой (модель _Настя_) [2a153ecb-b464-4892-a553-86f510d6a8ef].mp3",
			"2a153ecb-b464-4892-a553-86f510d6a8ef"},
		{"Replace 01_24-01_26 (04_03 AM Jul 28) [7f1473fe-a2c7-4d1b-8d2f-18a7d1e8cd1f].mp3",
			"7f1473fe-a2c7-4d1b-8d2f-18a7d1e8cd1f"},
		{"Гудки (Edit) [29b22cdc-7f9a-4f62-9c4f-5ba5322a1120].mp3",
			"29b22cdc-7f9a-4f62-9c4f-5ba5322a1120"},
		{"no id here.mp3", ""},
		{"badid [12345].mp3", ""},
	}
	for _, c := range cases {
		if got := trackIDFromFilename(c.name); got != c.want {
			t.Errorf("trackIDFromFilename(%q) = %q, want %q", c.name, got, c.want)
		}
	}
}
