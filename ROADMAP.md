# Roadmap

## MVP 1.0 ✅ (Done)

### Phase 1 — Infrastructure
- [x] Python analyzer structure (`analyze.py`, `utils/audio.py`)
- [x] Go orchestrator (`internal/analyzer/`) — subprocess bridge
- [x] SQLite `analysis_results` table (JSON blob storage)
- [x] Web UI: "Analyze" button, progress polling, metric bars
- [x] `Makefile` — `setup-python` target
- [x] CLI `version` command

### Phase 2 — Core Audio Metrics
- [x] **Loudness**: LUFS, True Peak, Crest Factor, Dynamic Range
- [x] **Phase**: Correlation, Mono Compatibility, Bass Phase, Stereo Stability
- [x] **Temporal**: BPM, Key Detection, Transient Integrity
- [x] **Spectral**: Balance, Band Energy, Conflict Detection (5 zones)

### Phase 3 — AI + Recommendations
- [x] **Whisper**: faster-whisper transcription, language detection
- [x] **Lyrics Comparison**: Jaccard similarity vs Suno lyrics
- [x] **Recommendation Engine**: 15+ rules across all metrics, severity scoring, mix quality label

### Phase 4 — Translation Readiness + Streaming Score
- [x] **7 Device Profiles**: iPhone Speaker, Samsung, AirPods, Car Audio, Bluetooth Speaker, Laptop, Club System
- [x] **5 Platform Checks**: Spotify, Apple Music, YouTube Music, Amazon Music, Tidal
- [x] Loudness penalty calculation, True Peak compliance

---

## MVP 2.0 — Professional Features

### Reference Matching Engine ✅
- [x] Load reference track (select from synced tracks or upload), compare EQ curve, stereo image, dynamics
- [x] Similarity scores per domain (atmosphere, mix, energy, stereo)
- [x] "Your mix vs reference" visual overlay (comparison chart with matplotlib)

### Hook & Structure Analysis
- [ ] Section detection (intro, verse, chorus, bridge, outro)
- [ ] Hook strength scoring
- [ ] Retention curve (listener attention over time)
- [ ] Energy envelope visualization

### Audio Visualizations
- [ ] Spectrogram rendering (PNG from Python → served by Go)
- [ ] Waveform overview
- [ ] Frequency distribution charts

### Batch Analysis
- [ ] Analyze all tracks in workspace
- [ ] Compare tracks side-by-side
- [ ] Library-wide statistics dashboard

---

## MVP 3.0 — AI Mastering Assistant

### AI Mastering Advisor
- [ ] EQ suggestions based on spectral conflicts
- [ ] Compression/limiting recommendations
- [ ] Stereo widening advice
- [ ] Genre-specific presets

### Commercial Potential Scoring
- [ ] Streaming readiness combined score
- [ ] Production quality assessment
- [ ] Genre compliance check

### A&R Assistant
- [ ] Automatic tagging and categorization
- [ ] Similar track clustering
- [ ] Trend analysis across library

---

## Technical Debt & Infrastructure

- [ ] Add proper test coverage for Python metrics
- [ ] CI/CD pipeline (GitHub Actions)
- [ ] Docker image for easy deployment
- [ ] Python analyzer as standalone HTTP microservice (FastAPI)
- [ ] GPU acceleration for Whisper (CUDA support)
- [ ] WebSocket for real-time analysis progress

---

## Changelog

### 2026-07-28 — v1.0.0
- All MVP 1.0 phases complete
- 8 metric groups, Go+Python hybrid architecture
- Chrome extension for auth token management

### 2026-07-28 — v1.1.0
- Reference Matching Engine: compare any track against a reference (synced or uploaded)
- 4 domain scores: atmosphere, mix, energy, stereo
- Visual comparison chart (per-band EQ + dynamic envelope overlay)
- Compare via dropdown (synced tracks) or file upload
