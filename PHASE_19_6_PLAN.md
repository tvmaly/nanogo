# Phase 19.6 Plan: Tool-Aware Voice Chat

## Summary

Phase 19.6 makes `nanogo voice chat` use the full nanogo agent loop. Final STT transcripts are appended to a persistent live voice session, routed through `core/agent`, allowed to call normal nanogo tools, and then spoken through the active TTS provider.

OpenRouter web search is available to voice chat by default as a provider-side server tool. The voice system prompt instructs the model to use web search only when explicitly asked to search/browse/look up information or when answering requires current/latest facts.

## Key Changes

- Replace the direct voice `provider.Chat` wrapper with an agent-backed `voice.AgentGateway`.
- Keep `modules/voice` responsible for STT/TTS orchestration and final-transcript handling.
- Build normal runtime tools for voice turns using the existing CLI composition path.
- Exclude `ask_user` from voice chat until a voice-native ask/answer flow exists.
- Inject OpenRouter `openrouter:web_search` into voice OpenAI/OpenRouter requests by default.
- Add optional `voice.web_search` config for engine, result limits, context size, and domain allow/exclude lists.
- Preserve existing function tools and existing OpenRouter server tools.

## Usage

```sh
make apple-voice-helpers
GOCACHE=/tmp/go-cache go build -tags malgo -o /tmp/nanogo ./cmd/nanogo
/tmp/nanogo voice chat --stt apple --tts apple --locale en-US --debug
```

Example prompts:

- "What tools do you have available in this voice session?"
- "Use a tool to list the files in this repository root, then summarize what you found."
- "Search the web for the latest OpenAI model news and summarize the top results."

## Verification

- `GOCACHE=/tmp/go-cache go test ./cmd/nanogo ./modules/voice/... ./ext/llm/openai/...`
- `make verify-local`
- Manual OpenRouter smoke with live Apple STT/TTS.
