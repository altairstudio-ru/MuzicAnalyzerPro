import os
import numpy as np
from faster_whisper import WhisperModel

_MODEL = None


def _get_model():
    global _MODEL
    if _MODEL is None:
        model_size = os.environ.get("WHISPER_MODEL", "base")
        _MODEL = WhisperModel(model_size, device="auto", compute_type="int8")
    return _MODEL


def measure(audio: np.ndarray, sr: int, original_lyrics: str = "") -> dict:
    try:
        model = _get_model()
        audio_float = audio.astype(np.float32)
        if audio_float.shape[0] > 1:
            audio_float = np.mean(audio_float, axis=0)
        segments, info = model.transcribe(audio_float, beam_size=3)
        segments_list = list(segments)
        full_text = " ".join(s.text.strip() for s in segments_list if s.text)
        segments_out = [
            {
                "start": round(s.start, 2),
                "end": round(s.end, 2),
                "text": s.text.strip(),
            }
            for s in segments_list
            if s.text
        ]
        similarity = _text_similarity(full_text, original_lyrics) if original_lyrics else None
        return {
            "language": info.language,
            "language_probability": round(info.language_probability, 2),
            "duration": round(info.duration, 1) if info.duration else 0,
            "full_text": full_text,
            "segments": segments_out[:50],
            "segment_count": len(segments_out),
            "original_lyrics": original_lyrics if original_lyrics else None,
            "lyrics_similarity": similarity,
        }
    except Exception as e:
        return {
            "error": str(e),
            "language": "unknown",
            "full_text": "",
            "segments": [],
        }


def _text_similarity(a: str, b: str) -> dict:
    a_words = set(a.lower().split())
    b_words = set(b.lower().split())
    if not a_words or not b_words:
        return {"word_overlap": 0, "note": "one or both texts are empty"}
    intersection = a_words & b_words
    union = a_words | b_words
    jaccard = len(intersection) / max(len(union), 1)
    return {
        "jaccard_similarity": round(jaccard, 3),
        "whisper_word_count": len(a_words),
        "original_word_count": len(b_words),
        "matching_words": len(intersection),
    }
