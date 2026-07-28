#!/usr/bin/env python3
"""
Suno Archiver — Audio Analysis Engine

CLI entry point. Called by Go via subprocess.
Reads an audio file, runs requested metrics, outputs JSON to stdout.

Usage:
    python3 analyze.py --input track.mp3 --metrics loudness,phase
    python3 analyze.py --input track.mp3 --metrics all
"""

import argparse
import json
import sys
import time

from utils.audio import load_audio
from metrics import loudness, phase

METRICS = {
    "loudness": loudness,
    "phase": phase,
}


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="MuzicAnalyzerPro audio analysis engine")
    parser.add_argument("--input", "-i", required=True, help="Path to audio file")
    parser.add_argument("--metrics", "-m", default="all",
                        help="Comma-separated metrics to run (default: all)")
    return parser.parse_args()


def run_all(yam: dict) -> dict:
    result = {"yam": yam}
    result["metrics"] = yam.get("requested", list(METRICS.keys()))
    result["results"] = {}
    errors = []
    for name, mod in METRICS.items():
        if yam.get("requested") and name not in yam["requested"]:
            continue
        try:
            result["results"][name] = mod.measure(yam["audio"], yam["sr"])
        except Exception as e:
            errors.append({"metric": name, "error": str(e)})
    if errors:
        result["errors"] = errors
    return result


def main():
    args = parse_args()
    requested = [m.strip() for m in args.metrics.split(",")] if args.metrics != "all" else list(METRICS.keys())
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
