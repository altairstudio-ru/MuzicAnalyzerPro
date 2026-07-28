import numpy as np


def measure(audio: np.ndarray, sr: int) -> dict:
    if audio.shape[0] < 2:
        return {
            "phase_correlation": 0.0,
            "mono_compatibility": 0.0,
            "bass_phase_alignment": 0.0,
            "stereo_stability": 0.0,
            "note": "mono input — phase metrics not applicable",
        }
    left = audio[0]
    right = audio[1]
    n = min(len(left), len(right))
    left = left[:n]
    right = right[:n]
    corr = float(np.corrcoef(left, right)[0, 1])
    phase_corr = max(-1, min(1, corr))
    mono = left + right
    mono_level = float(np.sqrt(np.mean(mono ** 2)))
    side = left - right
    side_level = float(np.sqrt(np.mean(side ** 2)))
    mono_compat = 10.0
    if side_level > 1e-10:
        ratio = mono_level / side_level
        mono_compat = min(10, max(0, float(ratio) * 2.0))
    bass_sr = min(sr, 200)
    bass_len = int(n * bass_sr / sr)
    if bass_len > 0:
        bass_left = left[:bass_len]
        bass_right = right[:bass_len]
        bass_corr = float(np.corrcoef(bass_left, bass_right)[0, 1]) if bass_len > 1 else 1.0
    else:
        bass_corr = 1.0
    bass_phase = max(-1, min(1, bass_corr))
    chunk_size = sr // 10
    if chunk_size > 0 and n > chunk_size:
        num_chunks = n // chunk_size
        chunk_corrs = []
        for i in range(num_chunks):
            s, e = i * chunk_size, (i + 1) * chunk_size
            c = float(np.corrcoef(left[s:e], right[s:e])[0, 1])
            chunk_corrs.append(c)
        stability = 1.0 - float(np.std(chunk_corrs)) if chunk_corrs else 0.0
    else:
        stability = 0.0
    return {
        "phase_correlation": round(phase_corr * 5 + 5, 2),
        "mono_compatibility": round(min(10, max(0, mono_compat)), 2),
        "bass_phase_alignment": round(bass_phase * 5 + 5, 2),
        "stereo_stability": round(min(10, max(0, stability * 10)), 2),
        "raw_correlation": round(phase_corr, 4),
    }
