import soundfile as sf
import numpy as np


def load_audio(path: str, sr: int = 44100) -> tuple[np.ndarray, int]:
    data, orig_sr = sf.read(path)
    if len(data.shape) > 1:
        data = data.T
    else:
        data = np.array([data])
    if orig_sr != sr:
        from scipy.signal import resample
        num_samples = int(data.shape[1] * sr / orig_sr)
        data = resample(data, num_samples, axis=1)
    return data, sr


def load_mono(path: str, sr: int = 44100) -> np.ndarray:
    data, _ = load_audio(path, sr)
    if data.shape[0] > 1:
        data = np.mean(data, axis=0, keepdims=True)
    return data.flatten()
