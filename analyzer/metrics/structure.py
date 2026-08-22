import numpy as np
import librosa

MIN_SEGMENT_SECONDS = 8.0
GROUP_SIM_THRESHOLD = 0.90


def measure(audio: np.ndarray, sr: int) -> dict:
    mono = np.mean(audio, axis=0) if audio.shape[0] > 1 else audio.flatten()
    duration = len(mono) / sr
    rms = librosa.feature.rms(y=mono)[0]
    if duration < 15 or len(rms) < 8:
        return _minimal_result(duration)
    try:
        sections = _detect_sections(mono, sr, duration, rms)
    except Exception:
        sections = [{
            "start": 0.0,
            "end": round(duration, 2),
            "label": "full",
            "energy": 0.5,
        }]
    hook = _hook_strength(sections, duration)
    return {
        "hook_score": hook["score"],
        "section_count": len(sections),
        "sections": sections,
        "hook_details": hook["details"],
        "retention_curve": _retention_curve(mono, sr),
        "energy_envelope": _energy_envelope(rms),
    }


def _minimal_result(duration: float) -> dict:
    return {
        "hook_score": 0.0,
        "section_count": 1,
        "sections": [{
            "start": 0.0,
            "end": round(duration, 2),
            "label": "full",
            "energy": 0.5,
        }],
        "hook_details": {
            "chorus_recurrence": 0,
            "distinctiveness": 0.0,
            "repetition_ratio": 0.0,
        },
        "retention_curve": [0.5] * 20,
        "energy_envelope": [0.5] * 20,
    }


def _detect_sections(mono: np.ndarray, sr: int, duration: float, rms: np.ndarray) -> list[dict]:
    feat = _timbre_features(mono, sr)
    idx = _sync_grid(mono, sr, feat.shape[1])
    feat_sync = librosa.util.sync(feat, idx, aggregate=np.mean)
    feat_sync = _zscore_rows(feat_sync)

    target_k = int(np.clip(round(duration / 15.0), 3, 10))
    cuts = _boundary_columns(feat_sync, target_k)
    seg_times = librosa.frames_to_time(idx, sr=sr)

    raw = []
    for i in range(len(cuts) - 1):
        c0, c1 = cuts[i], cuts[i + 1]
        start = float(seg_times[c0])
        end = min(float(seg_times[c1]) if c1 < len(seg_times) else duration, duration)
        if end > start:
            raw.append({"start": round(start, 2), "end": round(end, 2), "cols": (c0, c1)})
    raw = _merge_adjacent(raw, duration)
    if len(raw) == 1:
        s = raw[0]
        return [{"start": s["start"], "end": s["end"], "label": "full", "energy": 0.5}]
    if not raw:
        raise ValueError("no segments")

    energies = []
    for s in raw:
        f0 = int(librosa.time_to_frames(s["start"], sr=sr))
        f1 = max(int(librosa.time_to_frames(min(s["end"], duration), sr=sr)), f0 + 1)
        energies.append(float(np.mean(rms[f0:f1])))
    max_e = max(energies)
    for s, e in zip(raw, energies):
        s["energy"] = round(float(np.clip(e / max(max_e, 1e-9), 0.0, 1.0)), 2)

    seg_feats = np.stack([
        np.mean(feat_sync[:, s["cols"][0]:s["cols"][1]], axis=1) for s in raw
    ])
    _assign_labels(raw, seg_feats, duration)

    return [
        {"start": s["start"], "end": s["end"], "label": s["label"], "energy": s["energy"]}
        for s in raw
    ]


def _timbre_features(mono: np.ndarray, sr: int) -> np.ndarray:
    mfcc = librosa.feature.mfcc(y=mono, sr=sr, n_mfcc=12)
    chroma = librosa.feature.chroma_stft(y=mono, sr=sr)
    return np.vstack([mfcc, chroma])


def _sync_grid(mono: np.ndarray, sr: int, n_frames: int) -> np.ndarray:
    onset_env = librosa.onset.onset_strength(y=mono, sr=sr)
    beats = np.array([], dtype=int)
    try:
        _, beats = librosa.beat.beat_track(onset_envelope=onset_env, sr=sr)
        beats = np.unique(np.asarray(beats, dtype=int))
    except Exception:
        pass
    if len(beats) >= 16:
        grid = beats[(beats > 0) & (beats < n_frames)]
    else:
        step = max(int(round(sr / 512.0)), 1)
        grid = np.arange(step, n_frames, step, dtype=int)
    return np.concatenate([[0], grid])


def _zscore_rows(x: np.ndarray) -> np.ndarray:
    mu = x.mean(axis=1, keepdims=True)
    sd = x.std(axis=1, keepdims=True)
    sd[sd < 1e-9] = 1.0
    return (x - mu) / sd


def _boundary_columns(feat_sync: np.ndarray, k: int) -> list[int]:
    n_cols = feat_sync.shape[1]
    interior = set()
    try:
        bounds = librosa.segment.agglomerative(feat_sync, k)
        for b in np.atleast_1d(bounds):
            bi = int(b)
            if 0 < bi < n_cols:
                interior.add(bi)
    except Exception:
        pass
    if len(interior) < 2 and n_cols > 4:
        step = n_cols / float(k)
        for i in range(1, k):
            interior.add(int(round(i * step)))
    return sorted({0} | interior | {n_cols})


def _merge_adjacent(segments: list[dict], duration: float) -> list[dict]:
    merged = []
    for s in segments:
        if merged and merged[-1]["end"] - merged[-1]["start"] < MIN_SEGMENT_SECONDS:
            prev = merged[-1]
            prev["end"] = s["end"]
            c0, _ = prev["cols"]
            prev["cols"] = (c0, s["cols"][1])
        else:
            merged.append(s)
    if merged:
        merged[-1]["end"] = round(duration, 2)
    return merged


def _assign_labels(sections: list[dict], feats: np.ndarray, duration: float) -> None:
    n = len(sections)
    labels = ["section"] * n

    norms = np.linalg.norm(feats, axis=1, keepdims=True)
    norms[norms < 1e-9] = 1.0
    unit = feats / norms

    groups: list[list[int]] = []
    for i in range(n):
        placed = False
        for g in groups:
            ref = unit[g].mean(axis=0)
            ref /= max(np.linalg.norm(ref), 1e-9)
            if float(np.dot(unit[i], ref)) >= GROUP_SIM_THRESHOLD:
                g.append(i)
                placed = True
                break
        if not placed:
            groups.append([i])

    def g_total_dur(g):
        return sum(sections[i]["end"] - sections[i]["start"] for i in g)

    ranked = sorted(groups, key=lambda g: (-len(g), -g_total_dur(g)))
    chorus = ranked[0] if len(ranked[0]) >= 2 else None
    verse = next((g for g in ranked[1:] if len(g) >= 2), None)

    if chorus:
        for i in chorus:
            labels[i] = "chorus"
    if verse and verse is not chorus:
        for i in verse:
            if labels[i] == "section":
                labels[i] = "verse"

    if chorus:
        chorus_pos = sorted(chorus)
        for g in groups:
            if len(g) != 1:
                continue
            i = g[0]
            has_prev = any(c < i for c in chorus_pos)
            has_next = any(c > i for c in chorus_pos)
            if has_prev and has_next and labels[i] == "section":
                labels[i] = "bridge"

    med_e = float(np.median([s["energy"] for s in sections]))
    d0 = sections[0]["end"] - sections[0]["start"]
    if labels[0] != "chorus" and (sections[0]["energy"] < med_e or d0 <= 0.2 * duration):
        labels[0] = "intro"
    dn = sections[n - 1]["end"] - sections[n - 1]["start"]
    if labels[n - 1] != "chorus" and (sections[n - 1]["energy"] < med_e or dn <= 0.2 * duration):
        labels[n - 1] = "outro"

    for i, lab in enumerate(labels):
        sections[i]["label"] = lab


def _hook_strength(sections: list[dict], duration: float) -> dict:
    counts: dict[str, int] = {}
    dur_by_label: dict[str, float] = {}
    for s in sections:
        lab = s.get("label", "section")
        counts[lab] = counts.get(lab, 0) + 1
        dur_by_label[lab] = dur_by_label.get(lab, 0.0) + (s["end"] - s["start"])

    recurrence = counts.get("chorus", 0)
    repetition_ratio = min(dur_by_label.get("chorus", 0.0) / max(duration, 1e-9), 1.0)

    chorus_secs = [s for s in sections if s.get("label") == "chorus"]
    other_secs = [s for s in sections if s.get("label") != "chorus"]
    distinctiveness = 0.0
    if len(chorus_secs) >= 2:
        ch = np.array([s["energy"] for s in chorus_secs], dtype=float)
        spread_intra = float(np.mean(np.abs(ch - ch.mean())))
        if other_secs:
            ot = np.array([s["energy"] for s in other_secs], dtype=float)
            spread_cross = float(np.mean(np.abs(ch.mean() - ot)))
            distinctiveness = float(np.clip((spread_cross - spread_intra) * 4.0, 0.0, 1.0))

    recurrence_norm = float(np.clip((recurrence - 1) / 3.0, 0.0, 1.0))
    score = 10.0 * (
        0.35 * recurrence_norm
        + 0.30 * distinctiveness
        + 0.35 * repetition_ratio
    )
    return {
        "score": round(float(np.clip(score, 0.0, 10.0)), 2),
        "details": {
            "chorus_recurrence": recurrence,
            "distinctiveness": round(distinctiveness, 2),
            "repetition_ratio": round(repetition_ratio, 2),
        },
    }


def _retention_curve(mono: np.ndarray, sr: int, points: int = 100) -> list[float]:
    rms = librosa.feature.rms(y=mono)[0]
    if len(rms) < 4:
        return [0.5] * points
    win = max(len(rms) // 50, 3) | 1
    kernel = np.ones(win) / win
    env = np.convolve(rms, kernel, mode="same")
    nov = np.abs(np.diff(env, prepend=env[0]))
    nov = np.convolve(nov, kernel, mode="same")
    e_n = env / max(float(env.max()), 1e-9)
    n_n = nov / max(float(nov.max()), 1e-9)
    attention = 0.85 * e_n + 0.15 * n_n
    pos = np.linspace(0, len(attention) - 1, points)
    curve = np.interp(pos, np.arange(len(attention)), attention)
    return [round(float(v), 3) for v in np.clip(curve, 0.0, 1.0)]


def _energy_envelope(rms: np.ndarray, points: int = 200) -> list[float]:
    if len(rms) < 4:
        return [0.5] * points
    win = max(len(rms) // 100, 3) | 1
    kernel = np.ones(win) / win
    env = np.convolve(rms, kernel, mode="same")
    env = env / max(float(env.max()), 1e-9)
    pos = np.linspace(0, len(env) - 1, points)
    out = np.interp(pos, np.arange(len(env)), env)
    return [round(float(v), 3) for v in np.clip(out, 0.0, 1.0)]
