# Phase 22 Fixture Setup

Schema: `phase22.fixture_setup.v1`

Phase 22 deterministic tests use non-sensitive fixture frames under `testdata/phase22/`. Do not check in real child clips.

## Fixture Media Rules

- Store frame fixtures as small text or image files that do not identify a child.
- Store metadata in `testdata/phase22/frames_manifest.json`.
- Use local-only overrides for real clips during manual smoke tests.
- Never download or persist third-party video content; lesson research stores embeddable IDs, URLs, timestamp ranges, provenance, and summaries only.

## Manual Smoke Inputs

- `OPENROUTER_API_KEY` is required for live OpenRouter requests.
- `VISION_MODEL` selects the multimodal model for fixture-frame observations.
- `FFMPEG` may point to a local ffmpeg binary; otherwise `ffmpeg` is resolved from `PATH`.

## Expected Layout

```text
testdata/phase22/
  frames_manifest.json
  frames/
    throw-pass-001.txt
    throw-miss-001.txt
```
