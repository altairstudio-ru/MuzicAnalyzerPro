import numpy as np


def _val(results: dict, *path: str, default: float = 0) -> float:
    try:
        v = results
        for p in path:
            v = v[p]
        return float(v)
    except (KeyError, TypeError, IndexError, ValueError):
        return default


def measure(audio: np.ndarray, sr: int, all_results: dict = None,
            whisper_result: dict = None) -> dict:
    recs = []
    issues = []
    scores = {}
    r = all_results or {}
    loudness = r.get("loudness", {})
    phase = r.get("phase", {})
    temporal = r.get("temporal", {})
    spectral = r.get("spectral", {})
    lufs = _val(loudness, "lufs_integrated")
    if lufs > -8:
        recs.append({
            "area": "loudness",
            "severity": "warning",
            "message": f"Track is very loud ({lufs} LUFS). Consider reducing level to avoid streaming platform penalties.",
            "detail": "Spotify normalizes to -14 LUFS, Apple Music to -16 LUFS.",
        })
    elif lufs < -20:
        recs.append({
            "area": "loudness",
            "severity": "info",
            "message": f"Track is quiet ({lufs} LUFS). May benefit from gentle limiting.",
            "detail": "Typical commercial targets: -14 to -9 LUFS.",
        })
    dynamics = _val(loudness, "dynamic_range")
    if dynamics < 1.0:
        recs.append({
            "area": "dynamics",
            "severity": "warning",
            "message": f"Very low dynamic range ({dynamics}). Possible over-compression or limiting damage.",
            "detail": "Low dynamics cause listening fatigue. Consider reducing compression ratio.",
        })
    phase_corr = _val(phase, "phase_correlation")
    if phase_corr < 6:
        recs.append({
            "area": "phase",
            "severity": "critical",
            "message": f"Poor phase correlation ({phase_corr}/10). Mono compatibility may be compromised.",
            "detail": "Check stereo widening plugins and ensure bass is in mono.",
        })
    mono = _val(phase, "mono_compatibility")
    if mono < 6:
        recs.append({
            "area": "phase",
            "severity": "critical",
            "message": f"Low mono compatibility ({mono}/10). Bass may disappear on phone speakers.",
            "detail": "Use mid-side EQ to centre bass frequencies below 150 Hz.",
        })
    bass_phase = _val(phase, "bass_phase_alignment")
    if bass_phase < 7:
        recs.append({
            "area": "phase",
            "severity": "warning",
            "message": f"Bass phase alignment ({bass_phase}/10) could be improved.",
            "detail": "Check for anti-phase elements in kick and bass relationship.",
        })
    bpm_val = _val(temporal, "bpm")
    if bpm_val > 0:
        scores["bpm"] = bpm_val
    limiter = _val(temporal, "limiter_damage")
    if limiter < 4:
        recs.append({
            "area": "dynamics",
            "severity": "warning",
            "message": f"Limiter damage detected ({limiter}/10). Transient quality may be degraded.",
            "detail": "Reduce limiting threshold or use softer clipping.",
        })
    punch = _val(temporal, "drum_punch")
    if punch < 4:
        recs.append({
            "area": "dynamics",
            "severity": "info",
            "message": f"Low drum punch ({punch}/10). Transients may be too soft.",
            "detail": "Try parallel compression on drums or transient shaping.",
        })
    spectral_score = _val(spectral, "spectral_balance_score")
    scores["spectral_balance"] = spectral_score
    if spectral_score < 4:
        recs.append({
            "area": "spectral",
            "severity": "critical",
            "message": f"Spectral balance is poor ({spectral_score}/10). Mix may sound unbalanced.",
            "detail": "Check for masking issues. Consider referencing a professional mix.",
        })
    elif spectral_score < 6:
        recs.append({
            "area": "spectral",
            "severity": "warning",
            "message": f"Spectral balance could be improved ({spectral_score}/10).",
            "detail": "Review EQ balance across frequency bands.",
        })
    conflicts = spectral.get("conflicts", [])
    for c in conflicts:
        if c.get("severity") == "critical":
            zone = c.get("zone", "unknown")
            recs.append({
                "area": "spectral",
                "severity": "critical",
                "message": f"Critical masking in \"{zone}\" — energy excess of {c.get('excess_db', '?')} dB.",
                "detail": "Use EQ to carve space or adjust levels between conflicting elements.",
            })
            issues.append(zone)
    s_rolloff = _val(spectral, "spectral_rolloff")
    if s_rolloff < 200:
        recs.append({
            "area": "spectral",
            "severity": "info",
            "message": f"Low spectral rolloff ({s_rolloff} Hz). Mix may sound dull.",
            "detail": "Add high-frequency content or reduce low-mid buildup.",
        })
    masking_ratio = _val(spectral, "masking_ratio")
    if masking_ratio > 6:
        recs.append({
            "area": "spectral",
            "severity": "warning",
            "message": f"High masking ratio ({masking_ratio}/10). Elements may not have enough frequency space.",
            "detail": "Use spectrum analyser to identify clashing frequencies.",
        })
    if whisper_result:
        ws = whisper_result.get("lyrics_similarity")
        if ws and ws.get("jaccard_similarity", 1) < 0.3:
            recs.append({
                "area": "lyrics",
                "severity": "info",
                "message": "Low similarity between Suno lyrics and transcribed audio.",
                "detail": "Suno may have generated different lyrics than displayed. Check manually.",
            })
    overall_score = _compute_overall(recs, scores)
    recs.sort(key=lambda r: {"critical": 0, "warning": 1, "info": 2}.get(r["severity"], 3))
    n_issues = sum(1 for r in recs if r["severity"] == "critical")
    return {
        "overall_score": round(overall_score, 1),
        "recommendation_count": len(recs),
        "critical_issues": n_issues,
        "recommendations": recs,
        "mix_quality": _mix_quality_label(overall_score),
    }


def _compute_overall(recs: list, scores: dict) -> float:
    base = 8.0
    for r in recs:
        if r["severity"] == "critical":
            base -= 1.5
        elif r["severity"] == "warning":
            base -= 0.6
        else:
            base -= 0.2
    return max(1.0, min(10.0, base))


def _mix_quality_label(score: float) -> str:
    if score >= 8:
        return "excellent"
    if score >= 6:
        return "good"
    if score >= 4:
        return "fair"
    return "poor"
