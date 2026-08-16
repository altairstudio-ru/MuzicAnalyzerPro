import numpy as np
import os
import librosa
import pyloudnorm as pyln
import matplotlib
matplotlib.use('Agg')
import matplotlib.pyplot as plt

from utils.audio import load_audio

BANDS = [
    ("sub", 20, 60),
    ("bass", 60, 250),
    ("low_mid", 250, 500),
    ("mid", 500, 2000),
    ("high_mid", 2000, 4000),
    ("top", 4000, 20000),
]


def measure(audio: np.ndarray, sr: int, reference_path: str = None, all_results: dict = None) -> dict:
    if reference_path is None or not os.path.exists(reference_path):
        return {
            "error": "reference_path not provided or file not found",
            "spectral_similarity": 0,
            "domain_scores": {},
        }
    ref_audio, ref_sr = load_audio(reference_path, sr)
    min_len = min(audio.shape[1], ref_audio.shape[1])
    target = audio[:, :min_len]
    reference = ref_audio[:, :min_len]

    spectral_sim, per_band_sim, target_bands, ref_bands = _compare_spectral(target, reference, sr)
    stereo_sim = _compare_stereo(target, reference)
    dynamic_sim, target_env, ref_env = _compare_dynamics(target, reference)
    loudness_diff = _compare_loudness(target, reference, sr)

    per_band_10 = {k: round(v * 10, 1) for k, v in per_band_sim.items()}
    spectral_10 = round(spectral_sim * 10, 1)

    domain_scores = {
        "atmosphere": round(min(10, max(0, spectral_10 * 0.5 + per_band_10.get("top", 5) * 0.3 + dynamic_sim * 10 * 0.2)), 1),
        "mix": round(spectral_10, 1),
        "energy": round(dynamic_sim * 10, 1),
        "stereo": round(stereo_sim * 10, 1),
    }

    comparison_plot = _generate_comparison_chart(target_bands, ref_bands, target_env, ref_env, sr)

    result = {
        "spectral_similarity": spectral_10,
        "per_band_similarity": per_band_10,
        "stereo_similarity": round(stereo_sim * 10, 1),
        "dynamic_similarity": round(dynamic_sim * 10, 1),
        "loudness_difference": round(loudness_diff, 2),
        "domain_scores": domain_scores,
        "target_band_energy": {k: round(float(v), 1) for k, v in zip([b[0] for b in BANDS], target_bands)},
        "reference_band_energy": {k: round(float(v), 1) for k, v in zip([b[0] for b in BANDS], ref_bands)},
    }
    if comparison_plot:
        result["comparison_plot"] = comparison_plot
    return result


def _band_energy(spec: np.ndarray, freqs: np.ndarray, low: float, high: float) -> float:
    mask = (freqs >= low) & (freqs < high)
    if not mask.any():
        return 1e-10
    return float(np.mean(spec[mask]))


def _compare_spectral(target: np.ndarray, reference: np.ndarray, sr: int):
    if target.shape[0] > 1:
        t_mono = np.mean(target, axis=0)
        r_mono = np.mean(reference, axis=0)
    else:
        t_mono = target.flatten()
        r_mono = reference.flatten()

    n_fft = 2048
    t_spec = np.abs(librosa.stft(t_mono, n_fft=n_fft))
    r_spec = np.abs(librosa.stft(r_mono, n_fft=n_fft))
    freqs = librosa.fft_frequencies(sr=sr, n_fft=n_fft)

    t_means = np.mean(t_spec, axis=1)
    r_means = np.mean(r_spec, axis=1)

    target_bands = []
    ref_bands = []
    per_band = {}
    for name, low, high in BANDS:
        te = _band_energy(t_means, freqs, low, high)
        re = _band_energy(r_means, freqs, low, high)
        target_bands.append(te)
        ref_bands.append(re)

        t_env = np.mean(t_spec[freqs >= low], axis=0) if np.any(freqs >= low) else np.zeros(t_spec.shape[1])
        r_env = np.mean(r_spec[freqs >= low], axis=0) if np.any(freqs >= low) else np.zeros(r_spec.shape[1])
        min_len = min(len(t_env), len(r_env))
        if min_len > 1:
            corr = np.corrcoef(t_env[:min_len], r_env[:min_len])[0, 1]
            per_band[name] = max(0, float(corr))
        else:
            per_band[name] = 0.0

    vec_t = np.array(target_bands)
    vec_r = np.array(ref_bands)
    norm_t = np.linalg.norm(vec_t)
    norm_r = np.linalg.norm(vec_r)
    if norm_t > 0 and norm_r > 0:
        spectral_sim = float(np.dot(vec_t, vec_r) / (norm_t * norm_r))
    else:
        spectral_sim = 0.0
    spectral_sim = max(0, min(1, spectral_sim))

    return spectral_sim, per_band, target_bands, ref_bands


def _compare_stereo(target: np.ndarray, reference: np.ndarray) -> float:
    def _phase_corr(sig: np.ndarray) -> float:
        if sig.shape[0] < 2:
            return 1.0
        L = sig[0]
        R = sig[1] if sig.shape[0] > 1 else L
        denom = (np.linalg.norm(L) * np.linalg.norm(R))
        if denom == 0:
            return 1.0
        return float(np.dot(L, R) / denom)

    tc = _phase_corr(target)
    rc = _phase_corr(reference)
    diff = abs(tc - rc)
    return max(0, 1.0 - diff)


def _compare_dynamics(target: np.ndarray, reference: np.ndarray) -> tuple:
    if target.shape[0] > 1:
        t = np.mean(target, axis=0)
        r = np.mean(reference, axis=0)
    else:
        t = target.flatten()
        r = reference.flatten()

    frame_len = 1024
    hop = 512
    t_env = np.array([
        np.sqrt(np.mean(t[i:i+frame_len]**2))
        for i in range(0, len(t) - frame_len, hop)
    ])
    r_env = np.array([
        np.sqrt(np.mean(r[i:i+frame_len]**2))
        for i in range(0, len(r) - frame_len, hop)
    ])

    min_len = min(len(t_env), len(r_env))
    if min_len > 1:
        corr = np.corrcoef(t_env[:min_len], r_env[:min_len])[0, 1]
        dynamic_sim = max(0, float(corr))
    else:
        dynamic_sim = 0.0

    return dynamic_sim, t_env[:min_len].tolist() if min_len > 0 else [], r_env[:min_len].tolist() if min_len > 0 else []


def _compare_loudness(target: np.ndarray, reference: np.ndarray, sr: int) -> float:
    def _lufs(sig: np.ndarray, s: int) -> float:
        if sig.shape[0] > 1:
            sig = np.mean(sig, axis=0)
        else:
            sig = sig.flatten()
        meter = pyln.Meter(s)
        try:
            return meter.integrated_loudness(sig)
        except Exception:
            return -float('inf')
    tl = _lufs(target, sr)
    rl = _lufs(reference, sr)
    if tl == -float('inf') or rl == -float('inf'):
        return 0.0
    return float(tl - rl)


def _generate_comparison_chart(target_bands: list, ref_bands: list,
                                target_env: list, ref_env: list,
                                sr: int) -> str:
    try:
        fig, axes = plt.subplots(1, 2, figsize=(10, 4), facecolor='#0b0b10')

        band_labels = [b[0].replace('_', ' ').title() for b in BANDS]

        x = np.arange(len(band_labels))
        w = 0.35

        ax = axes[0]
        ax.set_facecolor('#12121a')
        tb_db = [20 * np.log10(max(e, 1e-10)) for e in target_bands]
        rb_db = [20 * np.log10(max(e, 1e-10)) for e in ref_bands]
        offset = -np.mean(tb_db) if tb_db else 0
        tb_db = [v + offset for v in tb_db]
        rb_db = [v + offset for v in rb_db]

        ax.bar(x - w/2, tb_db, w, label='Target', color='#ffb454', alpha=0.9)
        ax.bar(x + w/2, rb_db, w, label='Reference', color='#8f8a83', alpha=0.9)
        ax.set_xticks(x)
        ax.set_xticklabels(band_labels, fontsize=8, color='#8f8a83')
        ax.set_ylabel('Level (dB rel.)', fontsize=9, color='#8f8a83')
        ax.set_title('EQ Curve Comparison', fontsize=11, color='#ece7e1')
        ax.legend(fontsize=8, facecolor='#12121a', labelcolor='#ece7e1')
        ax.tick_params(colors='#8f8a83')
        for spine in ax.spines.values():
            spine.set_color('#34343b')

        ax2 = axes[1]
        ax2.set_facecolor('#12121a')
        if target_env and ref_env:
            min_l = min(len(target_env), len(ref_env))
            t_norm = np.array(target_env[:min_l])
            r_norm = np.array(ref_env[:min_l])
            t_norm = t_norm / (np.max(t_norm) or 1)
            r_norm = r_norm / (np.max(r_norm) or 1)
            time_axis = np.arange(min_l) * 512 / sr
            ax2.plot(time_axis, t_norm, color='#ffb454', alpha=0.8, linewidth=1, label='Target')
            ax2.plot(time_axis, r_norm, color='#8f8a83', alpha=0.8, linewidth=1, label='Reference')
        ax2.set_xlabel('Time (s)', fontsize=9, color='#8f8a83')
        ax2.set_ylabel('Normalized RMS', fontsize=9, color='#8f8a83')
        ax2.set_title('Dynamic Envelope', fontsize=11, color='#ece7e1')
        ax2.legend(fontsize=8, facecolor='#12121a', labelcolor='#ece7e1')
        ax2.tick_params(colors='#8f8a83')
        for spine in ax2.spines.values():
            spine.set_color('#34343b')

        plt.tight_layout()
        plot_dir = os.environ.get('ANALYZER_PLOT_DIR', '/tmp/analyzer_plots')
        os.makedirs(plot_dir, exist_ok=True)
        plot_path = os.path.join(plot_dir, f'reference_{abs(hash(str(target_bands)))}.png')
        plt.savefig(plot_path, dpi=100, bbox_inches='tight', facecolor='#0b0b10')
        plt.close(fig)
        return plot_path
    except Exception as e:
        return ""
