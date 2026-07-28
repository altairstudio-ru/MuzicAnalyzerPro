import numpy as np
import pyloudnorm as pyln

PLATFORMS = {
    "spotify": {
        "label": "Spotify",
        "target_lufs": -14,
        "max_true_peak": -1.0,
        "loudness_penalty_threshold": -14,
        "description": "Normalizes to -14 LUFS, applies limiting above -1 dB True Peak",
    },
    "apple_music": {
        "label": "Apple Music",
        "target_lufs": -16,
        "max_true_peak": -1.0,
        "loudness_penalty_threshold": -16,
        "description": "Sound Check normalizes to -16 LUFS, -1 dB True Peak ceiling",
    },
    "youtube_music": {
        "label": "YouTube Music",
        "target_lufs": -14,
        "max_true_peak": -1.0,
        "loudness_penalty_threshold": -14,
        "description": "Normalizes to -14 LUFS, -1 dB True Peak",
    },
    "amazon_music": {
        "label": "Amazon Music",
        "target_lufs": -14,
        "max_true_peak": -1.0,
        "loudness_penalty_threshold": -14,
        "description": "Normalizes to -14 LUFS",
    },
    "tidal": {
        "label": "Tidal",
        "target_lufs": -14,
        "max_true_peak": -1.0,
        "loudness_penalty_threshold": -14,
        "description": "Normalizes to -14 LUFS, -1 dB True Peak",
    },
}


def _compute_loudness_penalty(lufs: float, target: float) -> float:
    if lufs <= target:
        return 0.0
    excess = lufs - target
    penalty = excess * 0.6
    return round(min(penalty, 6.0), 2)


def _score_platform(
    lufs: float,
    true_peak_db: float,
    mono_score: float,
    platform: dict,
) -> dict:
    loudness_penalty = _compute_loudness_penalty(lufs, platform["target_lufs"])
    loudness_ok = loudness_penalty < 1.0
    true_peak_ok = true_peak_db <= platform["max_true_peak"]
    mono_ok = mono_score >= 6.0
    issues = []
    if not loudness_ok:
        issues.append(
            f"Loudness penalty: -{loudness_penalty} dB ({lufs} vs target {platform['target_lufs']} LUFS)"
        )
    if not true_peak_ok:
        issues.append(
            f"True Peak {true_peak_db} dB exceeds limit {platform['max_true_peak']} dB"
        )
    if not mono_ok:
        issues.append("Mono compatibility may cause issues")
    score = 10.0
    if not loudness_ok:
        score -= 3.0
    if not true_peak_ok:
        score -= 1.5
    if not mono_ok:
        score -= 1.5
    score = max(0, min(10, score))
    readiness = score / 10 * 100
    expected_adjustment = f"{loudness_penalty:.1f} dB"
    if lufs <= platform["target_lufs"]:
        expected_adjustment = "0 dB (already compliant)"
    return {
        "score": round(score, 1),
        "readiness_percent": round(readiness, 0),
        "loudness_penalty_db": loudness_penalty,
        "expected_normalization": expected_adjustment,
        "true_peak_compliant": true_peak_ok,
        "mono_compliant": mono_ok,
        "issues": issues,
    }


def measure(audio: np.ndarray, sr: int, all_results: dict = None) -> dict:
    if audio.shape[0] > 1:
        mono_for_loudness = np.mean(audio, axis=0)
    else:
        mono_for_loudness = audio.flatten()
    meter = pyln.Meter(sr)
    try:
        lufs = meter.integrated_loudness(mono_for_loudness)
    except Exception:
        lufs = -14.0
    peak = float(np.max(np.abs(mono_for_loudness)))
    true_peak_db = round(20 * np.log10(peak), 2) if peak > 0 else -99.0
    mono_score = 10.0
    if all_results:
        phase_data = all_results.get("phase", {})
        mono_score = float(phase_data.get("mono_compatibility", 10))
    platforms = {}
    overall_scores = []
    for key, plat in PLATFORMS.items():
        result = _score_platform(lufs, true_peak_db, mono_score, plat)
        platforms[key] = result
        overall_scores.append(result["score"])
    avg_readiness = (
        round(float(np.mean(overall_scores) / 10 * 100), 0)
        if overall_scores
        else 50
    )
    return {
        "integrated_lufs": round(lufs, 2),
        "true_peak_db": true_peak_db,
        "mono_compatibility": round(mono_score, 1),
        "overall_readiness": avg_readiness,
        "loudest_platform": max(
            platforms, key=lambda k: platforms[k]["loudness_penalty_db"]
        ),
        "platforms": platforms,
    }
