import numpy as np
import librosa


FREQ_BANDS = [
    ("sub_bass", 20, 60),
    ("bass", 60, 250),
    ("low_mid", 250, 500),
    ("mid", 500, 2000),
    ("upper_mid", 2000, 4000),
    ("presence", 4000, 8000),
    ("brilliance", 8000, 20000),
]

CONFLICT_ZONES = [
    {"name": "vocal_clarity", "low": 2000, "high": 4000, "label": "Lead vocal clarity"},
    {"name": "bass_definition", "low": 60, "high": 200, "label": "Bass definition"},
    {"name": "low_mid_muddiness", "low": 200, "high": 500, "label": "Low-mid muddiness"},
    {"name": "presence_air", "low": 5000, "high": 10000, "label": "Presence / air"},
    {"name": "master_brightness", "low": 8000, "high": 16000, "label": "High frequency energy"},
]


def measure(audio: np.ndarray, sr: int) -> dict:
    mono = np.mean(audio, axis=0) if audio.shape[0] > 1 else audio.flatten()
    stft = np.abs(librosa.stft(mono))
    freqs = librosa.fft_frequencies(sr=sr)
    spectral_flatness = float(np.mean(librosa.feature.spectral_flatness(y=mono)))
    spec_bandwidth = float(np.mean(librosa.feature.spectral_bandwidth(y=mono, sr=sr)))
    spec_rolloff = float(np.mean(librosa.feature.spectral_rolloff(y=mono, sr=sr)))
    band_energy = _band_energy_distribution(stft, freqs, sr)
    conflicts = _detect_conflicts(stft, freqs, sr)
    masking_ratio = _compute_masking_ratio(stft, freqs, sr)
    overall_score = _overall_spectral_score(band_energy, conflicts, masking_ratio)
    return {
        "spectral_balance_score": round(overall_score, 2),
        "spectral_flatness": round(spectral_flatness, 4),
        "spectral_bandwidth": round(spec_bandwidth, 1),
        "spectral_rolloff": round(spec_rolloff, 1),
        "masking_ratio": round(masking_ratio, 2),
        "band_energy": {name: round(float(e), 2) for name, _, _, e in band_energy},
        "conflicts": [
            {
                "zone": z["label"],
                "excess_db": round(c, 1),
                "severity": "critical" if c > 6 else "warning" if c > 3 else "ok",
            }
            for z, c in zip(CONFLICT_ZONES, conflicts)
        ],
    }


def _band_energy_distribution(stft, freqs, sr):
    total = float(np.sum(stft))
    result = []
    for name, low, high in FREQ_BANDS:
        mask = (freqs >= low) & (freqs < high)
        energy = float(np.sum(stft[mask])) / max(total, 1e-10) * 100
        result.append((name, low, high, energy))
    return result


def _detect_conflicts(stft, freqs, sr):
    results = []
    for zone in CONFLICT_ZONES:
        mask = (freqs >= zone["low"]) & (freqs < zone["high"])
        energy = float(np.sum(stft[mask]))
        neighbors_low = (freqs >= max(zone["low"] * 0.5, 20)) & (freqs < zone["low"])
        neighbors_high = (freqs >= zone["high"]) & (freqs < min(zone["high"] * 1.5, 20000))
        neighbor_energy = float(np.sum(stft[neighbors_low])) + float(np.sum(stft[neighbors_high]))
        ratio = 0
        if neighbor_energy > 1e-10:
            ratio = energy / neighbor_energy
        excess = float(np.clip((ratio - 1.5) * 3, 0, 10))
        results.append(excess)
    return results


def _compute_masking_ratio(stft, freqs, sr):
    spec = np.mean(stft, axis=1)
    total = float(np.sum(spec))
    if total < 1e-10:
        return 0.0
    normalized = spec / total
    entropy = -float(np.sum(normalized * np.log2(normalized + 1e-10)))
    max_entropy = np.log2(len(normalized))
    uniformity = entropy / max_entropy if max_entropy > 0 else 0
    return float(np.clip((1.0 - uniformity) * 10, 0, 10))


def _overall_spectral_score(band_energy, conflicts, masking_ratio):
    energies = {name: e for name, _, _, e in band_energy}
    penalty = 0
    for _, _, _, e in band_energy:
        if e < 1:
            penalty += 1
        if e > 50:
            penalty += 1
    for c in conflicts:
        if c > 6:
            penalty += 2
        elif c > 3:
            penalty += 1
    if masking_ratio > 7:
        penalty += 2
    elif masking_ratio > 4:
        penalty += 1
    score = max(0, min(10, 10 - penalty))
    return score
