# Phase 19.6 Acceptance Tests

## Automated Acceptance

| ID | Test | Expected Result |
|---|---|---|
| P19.6-AT-01 | Voice final transcript uses full agent loop | Final STT text is appended to a nanogo session and answered through `core/agent`, not a direct `provider.Chat` wrapper. |
| P19.6-AT-02 | Voice agent executes tools | A model-emitted function tool call is dispatched through the normal tool runtime and the final answer is spoken through TTS. |
| P19.6-AT-03 | `ask_user` excluded | Voice chat tool list does not expose `ask_user` in Phase 19.6. |
| P19.6-AT-04 | Web search default injection | Voice chat OpenAI/OpenRouter config includes `openrouter:web_search` by default. |
| P19.6-AT-05 | Advanced web search config | `voice.web_search` passes through engine, limits, context size, allowed domains, and excluded domains. |
| P19.6-AT-06 | Router voice route injection | Router configs inject web search into the OpenAI provider selected by `source=voice` or default route without changing non-OpenAI providers. |
| P19.6-AT-07 | Existing tools preserved | Local function tools and existing OpenRouter server tools remain in the same request. |
| P19.6-AT-08 | Search policy prompt | Voice chat system prompt tells the model to search only on explicit/current-info requests. |
| P19.6-AT-09 | Audio privacy | Raw PCM is not persisted unless an explicit debug PCM flag is used. |

## Manual Acceptance

Run:

```sh
export OPENROUTER_API_KEY="sk-or-v1-..."
make apple-voice-helpers
GOCACHE=/tmp/go-cache go build -tags malgo -o /tmp/nanogo ./cmd/nanogo
/tmp/nanogo voice chat --stt apple --tts apple --locale en-US --debug
```

Ask:

- "What tools do you have available in this voice session?"
- "Use a tool to list the files in this repository root, then summarize what you found."
- "Search the web for the latest OpenAI model news and summarize the top results."
- "Explain magnets to a 10-year-old without searching the web."

Expected debug output includes:

```text
voice chat stt final="..."
voice chat agent reply="..."
voice chat tts audio bytes=...
voice chat tts done
```

For explicit search prompts, OpenRouter may report `web_search_requests` in usage.
