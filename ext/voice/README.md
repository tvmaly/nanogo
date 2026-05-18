# Voice Extension

`ext/voice` contains optional voice behavior outside the kernel. The normalized
runtime contract lives in `ext/voice/realtime`, session management lives in
`ext/voice/session`, provider adapters live under `ext/voice/providers`, and
local device support stays isolated in `ext/voice/localaudio`.

There are two voice capability shapes to keep separate:

- direct realtime voice-agent sessions, where the provider can produce text,
  audio, and function-call style events;
- STT/TTS transport-style sessions, where nanogo remains the thinking agent and
  speech providers only transcribe or synthesize.

Phase 17.5 keeps normalized event persistence enabled by default and makes raw
provider event logs explicit opt-in through session config. This keeps debugging
possible without writing provider payloads, transcripts, or tool arguments unless
a caller deliberately enables raw capture.

Relevant checks:

```sh
go test ./ext/voice/...
```
