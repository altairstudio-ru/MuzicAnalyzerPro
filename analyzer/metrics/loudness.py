import numpy as np
import pyloudnorm as pyln


def measure(audio: np.ndarray, sr: int) -> dict:
    if audio.shape[0] > 1:
        audio = np.mean(audio, axis=0)
    else:
        audio = audio.flatten()
    meter = pyln.Meter(sr)
    lufs = meter.integrated_loudness(audio)
    peak = float(np.max(np.abs(audio)))
    true_peak = float(np.max(np.abs(audio)))
    crest = 20 * np.log10(true_peak / (10 ** (lufs / 20))) if lufs != -float('inf') else 0
    dynamic_range = float(np.percentile(np.abs(audio), 95) - np.percentile(np.abs(audio), 5))
    return {
        "lufs_integrated": round(float(lufs), 2),
        "true_peak_db": round(20 * np.log10(true_peak), 2) if true_peak > 0 else -float('inf'),
        "crest_factor": round(float(crest), 2),
        "dynamic_range": round(dynamic_range, 2),
        "peak_amplitude": round(float(peak), 6),
    }
