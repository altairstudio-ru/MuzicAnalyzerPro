# MuzicAnalyzerPro / Suno Archiver

CLI + Web UI + Chrome Extension for archiving and analyzing Suno AI music tracks. Downloads tracks with full metadata, runs professional-grade audio analysis, and provides actionable mix recommendations.

## Features

- **Archive** — download Suno tracks with metadata (prompts, lyrics, tags)
- **Analyze** — 9 metric groups powered by Python audio engine
- **Reference Match** — compare any track against a reference (another Suno track or uploaded audio) across 4 domains: atmosphere, mix, energy, stereo
- **Visualize** — web UI with HTMX, dark theme, audio player, comparison charts
- **Recommend** — AI-powered improvement suggestions
- **Extend** — Chrome extension for one-click auth token extraction

## Architecture

```
┌─────────────────────────────────────────────────┐
│                  Go (CLI + Web)                  │
│  cmd/suno-archiver/    — cobra CLI              │
│  internal/                                     │
│    library/            — config, orchestration   │
│    db/                 — SQLite (tracks,         │
│                           workspaces, analysis)  │
│    suno/               — Suno API v3 client     │
│    analyzer/           — Python subprocess       │
│                           orchestrator           │
│    web/                — Chi router + HTMX       │
│                          templates               │
├─────────────────────────────────────────────────┤
│               Python (Analysis Engine)           │
│  analyzer/                                      │
│    analyze.py           — CLI entry point        │
│    utils/audio.py       — load, resample         │
│    metrics/                                     │
│      loudness.py        — LUFS, True Peak, DR   │
│      phase.py           — Correlation, Mono     │
│      temporal.py        — BPM, Key, Transients  │
│      spectral.py        — Masking, Conflicts    │
│      translation.py     — 7 device profiles     │
│      reference.py       — Spectral/stereo/       │
│                          dynamics comparison     │
│      streaming.py       — 5 platform checks     │
│    ai/                                          │
│      whisper.py         — faster-whisper        │
│      recommendations.py — Improvement engine    │
├─────────────────────────────────────────────────┤
│           Chrome Extension                       │
│  extensions/suno-archiver/                      │
│    content.js          — Clerk JWT extraction   │
│    popup.html/js       — token management       │
└─────────────────────────────────────────────────┘
```

## Quick Start

```bash
# 1. Setup Python environment
make setup-python

# 2. Build Go binary
make build

# 3. Save Suno auth token
./bin/suno-archiver auth <your-clerk-jwt>

# 4. Sync tracks
./bin/suno-archiver sync

# 5. Start web UI
./bin/suno-archiver serve
# → http://localhost:8080
```

Full web UI guide (pages, buttons, routes, troubleshooting):
**[docs/WEB_UI.md](docs/WEB_UI.md)**

## CLI Commands

| Command | Description |
|---------|-------------|
| `auth <token>` | Save Suno Clerk JWT |
| `sync` | Fetch all tracks, download audio |
| `serve` | Start web UI (default :8080) |
| `version` | Show version |
| `--path` | Custom library path (default `~/.muzicanalyzer`) |
| `--port` | Custom web port (default `:8080`) |

## Analysis Metrics

### Audio Metrics

| Metric | Source | What it measures |
|--------|--------|-----------------|
| **Loudness** | `pyloudnorm` | LUFS integrated, True Peak, Crest Factor, Dynamic Range |
| **Phase** | `numpy` | Phase Correlation, Mono Compatibility, Bass Phase Alignment, Stereo Stability |
| **Temporal** | `librosa` | BPM, Key Detection, Drum Punch, Vocal Attack, Limiter Damage, Micro-dynamics |
| **Spectral** | `librosa` | Spectral Balance, Band Energy Distribution, Frequency Conflict Detection (5 zones) |
| **Translation** | `librosa` | 7 device simulations: iPhone, Samsung, AirPods, Car Audio, Bluetooth Speaker, Laptop, Club System |
| **Reference Match** | `librosa` | Compare against reference: spectral (EQ) similarity, stereo image, dynamic envelope, loudness difference; 4 domain scores (atmosphere, mix, energy, stereo); visual comparison chart |
| **Streaming** | `pyloudnorm` | Spotify, Apple Music, YouTube Music, Amazon Music, Tidal readiness with loudness penalty |

### AI Metrics

| Metric | Source | What it does |
|--------|--------|-------------|
| **Whisper** | `faster-whisper` | Transcribes audio, detects language, compares with Suno lyrics (Jaccard similarity) |
| **Recommendations** | Rule engine | 15+ rules across all metrics with severity (critical/warning/info), mix quality score |

## Chrome Extension

The extension (`extensions/suno-archiver/`) automates auth token extraction:

1. Load unpacked extension in Chrome (`chrome://extensions` → Load unpacked → select `extensions/suno-archiver/`)
2. Open suno.com and log in
3. Extension auto-extracts Clerk JWT from localStorage/cookies
4. Auto-sends to local `suno-archiver serve` instance
5. Manual send and debug panel available in popup

## Development

```bash
make build           # Build Go binary
make run             # Run directly
make test            # Run Go tests
make fmt             # Format Go code
make clean           # Remove bin/ and tmp/
make setup-python    # Create .venv + install Python deps
```

Version is injected via `git describe` at build time:

```bash
make build VERSION=1.0.0
```

## Dependencies

**Go (module: `github.com/altairstudio-ru/MuzicAnalyzerPro`)**
- `go-chi/chi/v5` — HTTP router
- `spf13/cobra` — CLI framework
- `modernc.org/sqlite` — Pure Go SQLite (no CGO)
- `gopkg.in/yaml.v3` — YAML config

**Python (via `analyzer/requirements.txt`)**
- `librosa` — Audio analysis (BPM, key, spectral)
- `pyloudnorm` — LUFS, True Peak
- `faster-whisper` — Speech transcription
- `numpy` / `scipy` — Signal processing
- `soundfile` — Audio I/O

## Project Structure

```
├── analyzer/               # Python analysis engine
├── cmd/
│   └── suno-archiver/      # CLI entry point
├── extensions/
│   └── suno-archiver/      # Chrome extension
├── internal/
│   ├── analyzer/           # Go → Python orchestrator
│   ├── cli/                # (reserved)
│   ├── db/                 # SQLite CRUD
│   ├── library/            # Config + Manager
│   ├── suno/               # Suno API client
│   └── web/                # Web server + templates
├── pkg/models/             # Data models
├── bin/                    # Compiled binary (gitignored)
├── Makefile
└── go.mod
```
