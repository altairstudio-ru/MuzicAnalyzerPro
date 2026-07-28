#!/usr/bin/env python3
"""
Suno Archiver — Audio Analysis Engine

CLI entry point. Called by Go via subprocess.
Reads an audio file, runs requested metrics, outputs JSON to stdout.

Usage:
    python3 analyze.py --input track.mp3 --metrics loudness,phase
    python3 analyze.py --input track.mp3 --metrics all
    python3 analyze.py --input track.mp3 --lyrics "original song lyrics..."
"""

import argparse
import json
import sys
import time

from utils.audio import load_audio
from metrics import loudness, phase, temporal, spectral, translation, streaming
from ai import whisper, recommendations

METRICS = {
    "loudness": loudness,
    "phase": phase,
    "temporal": temporal,
    "spectral": spectral,
    "translation": translation,
}

AI_METRICS = {
    "whisper": whisper,
    "streaming": streaming,
    "recommendations": recommendations,
}


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="MuzicAnalyzerPro audio analysis engine")
    parser.add_argument("--input", "-i", required=True, help="Path to audio file")
    parser.add_argument("--lyrics", "-l", default="", help="Original Suno lyrics for comparison")
    parser.add_argument("--metrics", "-m", default="all",
                        help="Comma-separated metrics to run (default: all)")
    return parser.parse_args()


def run_all(yam: dict) -> dict:
    result = {"yam": yam}
    all_metric_names = list(METRICS.keys()) + list(AI_METRICS.keys())
    result["metrics"] = yam.get("requested", all_metric_names)
    result["results"] = {}
    errors = []
    requested = yam.get("requested", [])

    # Phase 1: standard metrics
    for name, mod in METRICS.items():
        if requested and name not in requested:
            continue
        try:
            result["results"][name] = mod.measure(yam["audio"], yam["sr"])
        except Exception as e:
            errors.append({"metric": name, "error": str(e)})

    # Phase 2: AI metrics (need previous results)
    for name, mod in AI_METRICS.items():
        if requested and name not in requested:
            continue
        try:
            if name == "whisper":
                result["results"][name] = mod.measure(
                    yam["audio"], yam["sr"],
                    original_lyrics=yam.get("lyrics", ""),
                )
            elif name == "streaming":
                result["results"][name] = mod.measure(
                    yam["audio"], yam["sr"],
                    all_results=result["results"],
                )
            elif name == "recommendations":
                whisper_result = result["results"].get("whisper")
                result["results"][name] = mod.measure(
                    yam["audio"], yam["sr"],
                    all_results=result["results"],
                    whisper_result=whisper_result,
                )
        except Exception as e:
            errors.append({"metric": name, "error": str(e)})

    if errors:
        result["errors"] = errors
    return result


def main():
    args = parse_args()
    all_names = list(METRICS.keys()) + list(AI_METRICS.keys())
    requested = [m.strip() for m in args.metrics.split(",")] if args.metrics != "all" else all_names
    t0 = time.time()
    try:
        audio, sr = load_audio(args.input)
    except Exception as e:
        print(json.dumps({"status": "error", "error": f"load audio: {e}"}))
        sys.exit(1)
    yam = {
        "audio": audio,
        "sr": sr,
        "requested": requested,
        "path": args.input,
        "lyrics": args.lyrics,
    }
    result = run_all(yam)
    elapsed = round(time.time() - t0, 2)
    output = {
        "status": "done",
        "elapsed_seconds": elapsed,
        "metrics": result["metrics"],
        "results": result["results"],
    }
    if result.get("errors"):
        output["errors"] = result["errors"]
    print(json.dumps(output, ensure_ascii=False, default=str))


if __name__ == "__main__":
    main()
