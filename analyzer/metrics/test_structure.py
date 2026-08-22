import os
import sys
import unittest

import numpy as np

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

from metrics import structure


def build_song(sr: int = 22050) -> np.ndarray:
    def tone(freqs, dur, amp):
        t = np.linspace(0, dur, int(sr * dur), endpoint=False)
        wave = sum(np.sin(2 * np.pi * f * t) for f in freqs)
        pulse = 0.5 * (1 + np.sign(np.sin(2 * np.pi * 2.0 * t)))
        return amp * wave * (0.6 + 0.4 * pulse)

    intro = tone([220], 8, 0.08)
    part_a = tone([330, 440], 14, 0.45)
    part_b = tone([550, 660, 880], 14, 0.35)
    part_a2 = part_a.copy()
    outro = tone([220], 10, 0.06)
    return np.concatenate([intro, part_a, part_b, part_a2, outro])


class TestStructure(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.sr = 22050
        cls.song = build_song(cls.sr)

    def test_full_result_shape(self):
        res = structure.measure(self.song.reshape(1, -1), self.sr)
        for key in (
            "hook_score", "section_count", "sections",
            "hook_details", "retention_curve", "energy_envelope",
        ):
            self.assertIn(key, res)
        self.assertGreaterEqual(res["section_count"], 2)
        self.assertEqual(len(res["sections"]), res["section_count"])
        self.assertEqual(len(res["retention_curve"]), 100)
        self.assertEqual(len(res["energy_envelope"]), 200)

    def test_sections_cover_track_and_ordered(self):
        res = structure.measure(self.song.reshape(1, -1), self.sr)
        secs = res["sections"]
        self.assertAlmostEqual(secs[0]["start"], 0.0, places=1)
        duration = len(self.song) / self.sr
        self.assertAlmostEqual(secs[-1]["end"], duration, delta=0.5)
        for a, b in zip(secs, secs[1:]):
            self.assertAlmostEqual(a["end"], b["start"], delta=0.15)
            self.assertGreater(b["end"], b["start"])
        valid = {"intro", "verse", "chorus", "bridge", "outro", "section", "full"}
        for s in secs:
            self.assertIn(s["label"], valid)
            self.assertTrue(0.0 <= s["energy"] <= 1.0)

    def test_repeated_part_detected_as_chorus(self):
        res = structure.measure(self.song.reshape(1, -1), self.sr)
        labels = [s["label"] for s in res["sections"]]
        chorus_count = sum(1 for l in labels if l == "chorus")
        self.assertGreaterEqual(chorus_count, 2)

    def test_hook_score_range_and_details(self):
        res = structure.measure(self.song.reshape(1, -1), self.sr)
        self.assertTrue(0.0 <= res["hook_score"] <= 10.0)
        d = res["hook_details"]
        self.assertIn("chorus_recurrence", d)
        self.assertIn("distinctiveness", d)
        self.assertIn("repetition_ratio", d)
        self.assertTrue(0.0 <= d["distinctiveness"] <= 1.0)
        self.assertTrue(0.0 <= d["repetition_ratio"] <= 1.0)

    def test_curves_in_unit_range(self):
        res = structure.measure(self.song.reshape(1, -1), self.sr)
        for v in res["retention_curve"]:
            self.assertTrue(0.0 <= v <= 1.0)
        for v in res["energy_envelope"]:
            self.assertTrue(0.0 <= v <= 1.0)
        self.assertAlmostEqual(max(res["energy_envelope"]), 1.0, places=2)

    def test_short_audio_minimal_result(self):
        short = np.sin(2 * np.pi * 440 * np.linspace(0, 5, self.sr * 5))
        res = structure.measure(short.reshape(1, -1), self.sr)
        self.assertEqual(res["section_count"], 1)
        self.assertEqual(res["sections"][0]["label"], "full")
        self.assertEqual(res["hook_score"], 0.0)


if __name__ == "__main__":
    unittest.main()
