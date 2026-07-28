import numpy as np
import librosa

PROFILES = {
    "iphone_speaker": {
        "label": "iPhone Speaker",
        "hp_cutoff": 350,
        "bass_boost_db": -12,
        "high_shelf_db": -3,
    },
    "samsung_speaker": {
        "label": "Samsung Speaker",
        "hp_cutoff": 300,
        "bass_boost_db": -10,
        "high_shelf_db": -2,
    },
    "airpods": {
        "label": "AirPods",
        "hp_cutoff": 80,
        "bass_boost_db": 2,
        "high_shelf_db": 1,
    },
    "car_audio": {
        "label": "Car Audio",
        "hp_cutoff": 40,
        "bass_boost_db": 5,
        "high_shelf_db": -2,
    },
    "bluetooth_speaker": {
        "label": "Bluetooth Speaker",
        "hp_cutoff": 180,
        "bass_boost_db": -4,
        "high_shelf_db": -1,
    },
    "laptop_speakers": {
        "label": "Laptop Speakers",
        "hp_cutoff": 250,
        "bass_boost_db": -8,
        "high_shelf_db": -3,
    },
    "club_system": {
        "label": "Club System",
        "hp_cutoff": 30,
        "bass_boost_db": 4,
        "high_shelf_db": 2,
    },
}


def _apply_profile(
    stft: np.ndarray, freqs: np.ndarray, profile: dict
) -> np.ndarray:
    modified = stft.copy()
    hp_mask = freqs < profile["hp_cutoff"]
    modified[hp_mask] *= 10 ** (profile.get("bass_boost_db", 0) / 20)
    if profile["hp_cutoff"] > 50:
        rolloff = np.exp(
            -((profile["hp_cutoff"] - freqs[hp_mask]) ** 2)
            / (2 * (profile["hp_cutoff"] / 2) ** 2)
        )
        modified[hp_mask] *= rolloff[:, None]
    high_idx = freqs > 8000
    if np.any(high_idx):
        db = profile.get("high_shelf_db", 0)
        gain = 10 ** (db / 20)
        modified[high_idx] *= gain
    return modified


def _score_profile(stft: np.ndarray, freqs: np.ndarray, sr: int) -> float:
    spec = np.mean(np.abs(stft), axis=1)
    total = float(np.sum(spec))
    if total < 1e-10:
        return 5.0
    norm = spec / total
    low = float(np.sum(norm[freqs < 150]))
    low_mid = float(np.sum(norm[(freqs >= 150) & (freqs < 500)]))
    mid = float(np.sum(norm[(freqs >= 500) & (freqs < 4000)]))
    high = float(np.sum(norm[freqs >= 4000]))
    penalty = 0
    if low > 0.5:
        penalty += 2
    if low_mid > 0.7:
        penalty += 1.5
    if mid < 0.1:
        penalty += 2
    if high < 0.02:
        penalty += 1
    if high > 0.5:
        penalty += 1
    score = max(0, min(10, 10 - penalty))
    return score


def _vocal_clarity_score(
    stft: np.ndarray, freqs: np.ndarray, sr: int
) -> float:
    spec = np.mean(np.abs(stft), axis=1)
    vocal_band = (freqs >= 1000) & (freqs < 4000)
    low_band = (freqs >= 100) & (freqs < 500)
    vocal_energy = float(np.sum(spec[vocal_band]))
    low_energy = float(np.sum(spec[low_band]))
    if low_energy < 1e-10:
        return 5.0
    ratio = vocal_energy / low_energy
    score = min(10, max(0, ratio * 5))
    return score


def _bass_presence(stft: np.ndarray, freqs: np.ndarray, sr: int) -> float:
    spec = np.mean(np.abs(stft), axis=1)
    bass = float(np.sum(spec[freqs < 150]))
    total = float(np.sum(spec))
    if total < 1e-10:
        return 5.0
    ratio = bass / total
    score = min(10, max(0, ratio * 40))
    return score


def measure(audio: np.ndarray, sr: int) -> dict:
    mono = np.mean(audio, axis=0) if audio.shape[0] > 1 else audio.flatten()
    stft = np.abs(librosa.stft(mono))
    freqs = librosa.fft_frequencies(sr=sr)
    results = {}
    overall_ratings = []
    for key, profile in PROFILES.items():
        sim_stft = _apply_profile(stft, freqs, profile)
        balance = _score_profile(sim_stft, freqs, sr)
        vocal = _vocal_clarity_score(sim_stft, freqs, sr)
        bass = _bass_presence(sim_stft, freqs, sr)
        device_score = round((balance * 0.4 + vocal * 0.35 + bass * 0.25), 2)
        results[key] = {
            "device": profile["label"],
            "score": device_score,
            "balance": round(balance, 2),
            "vocal_clarity": round(vocal, 2),
            "bass_presence": round(bass, 2),
        }
        overall_ratings.append(device_score)
    avg_score = round(float(np.mean(overall_ratings)), 2) if overall_ratings else 5.0
    worst_device = min(results, key=lambda k: results[k]["score"])
    best_device = max(results, key=lambda k: results[k]["score"])
    return {
        "overall_translation_score": avg_score,
        "worst_device": results[worst_device]["device"],
        "worst_score": results[worst_device]["score"],
        "best_device": results[best_device]["device"],
        "best_score": results[best_device]["score"],
        "profiles": results,
    }
