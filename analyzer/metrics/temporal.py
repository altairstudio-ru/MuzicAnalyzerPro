import numpy as np
import librosa
from librosa.feature.rhythm import tempo as tempo_feature


def measure(audio: np.ndarray, sr: int) -> dict:
    mono = np.mean(audio, axis=0) if audio.shape[0] > 1 else audio.flatten()
    onset_env = librosa.onset.onset_strength(y=mono, sr=sr)
    tempo = float(tempo_feature(onset_envelope=onset_env, sr=sr)[0])
    key_str, key_confidence = _detect_key(mono, sr)
    transient = _transient_integrity(mono, sr)
    return {
        "bpm": round(tempo, 1),
        "key": key_str,
        "key_confidence": round(key_confidence, 2),
        "drum_punch": round(transient["drum_punch"], 2),
        "vocal_attack": round(transient["vocal_attack"], 2),
        "limiter_damage": round(transient["limiter_damage"], 2),
        "micro_dynamics": round(transient["micro_dynamics"], 2),
    }


def _detect_key(y: np.ndarray, sr: int) -> tuple[str, float]:
    try:
        chroma = librosa.feature.chroma_cqt(y=y, sr=sr)
        mean_chroma = np.mean(chroma, axis=1)
        key_names = [
            "C", "C#", "D", "D#", "E", "F",
            "F#", "G", "G#", "A", "A#", "B",
        ]
        idx = int(np.argmax(mean_chroma))
        confidence = float(mean_chroma[idx] / np.sum(mean_chroma))
        return key_names[idx], confidence
    except Exception:
        return "unknown", 0.0


def _transient_integrity(y: np.ndarray, sr: int) -> dict:
    onset_frames = librosa.onset.onset_detect(y=y, sr=sr, backtrack=True)
    if len(onset_frames) < 2:
        return {"drum_punch": 5.0, "vocal_attack": 5.0, "limiter_damage": 5.0, "micro_dynamics": 5.0}
    onset_samples = librosa.frames_to_samples(onset_frames)
    attack_times = np.diff(onset_samples) / sr
    median_attack = float(np.median(attack_times)) if len(attack_times) > 0 else 0.1
    drum_punch = min(10, max(0, (0.05 / max(median_attack, 0.001)) * 5))
    rms = librosa.feature.rms(y=y)[0]
    rms_std = float(np.std(rms))
    rms_mean = float(np.mean(rms))
    crest = rms_std / max(rms_mean, 1e-10)
    vocal_attack = min(10, max(0, crest * 3))
    zcr = librosa.feature.zero_crossing_rate(y)
    zcr_std = float(np.std(zcr[0]))
    zcr_mean = float(np.mean(zcr[0]))
    zcr_variation = zcr_std / max(zcr_mean, 1e-10)
    limiter_damage = min(10, max(0, (1.0 - zcr_variation) * 6))
    spectral_centroids = librosa.feature.spectral_centroid(y=y, sr=sr)[0]
    centroid_std = float(np.std(spectral_centroids))
    centroid_mean = float(np.mean(spectral_centroids))
    centroid_cv = centroid_std / max(centroid_mean, 1e-10)
    micro_dynamics = min(10, max(0, centroid_cv * 15))
    return {
        "drum_punch": drum_punch,
        "vocal_attack": vocal_attack,
        "limiter_damage": limiter_damage,
        "micro_dynamics": micro_dynamics,
    }
