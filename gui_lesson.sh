#!/bin/bash
cat > /tmp/nanogo-webui-phase22.json <<EOF
  {
    "llm": {
      "driver": "openai",
      "config": {
        "base_url": "https://openrouter.ai/api/v1",
        "api_key_env": "OPENROUTER_API_KEY",
        "model": "anthropic/claude-haiku-4-5"
      }
    },
    "transports": [
      {
        "driver": "webui",
        "config": {
          "addr": "127.0.0.1:8090",
          "insecure_skip_auth": true,
          "lessons": [
            {
              "id": "fractions-browser",
              "title": "Fractions With Pizza",
              "blocks": [
                {
                  "id": "intro",
                  "kind": "prose",
                  "content": "A fraction is one part of a whole."
                },
                {
                  "id": "video",
                  "kind": "video",
                  "video_url": "https://www.youtube.com/embed/9McJ3GobPaY"
                },
                {
                  "id": "check",
                  "kind": "quiz",
                  "quiz_ref": "fractions-quick-check"
                }
              ]
            }
          ],
          "micro_lessons": [
            {
              "id": "ml-fractions-halves",
              "title": "Halves With Pizza",
              "child_safety_setup": "Use paper and crayons at a clear table.",
              "youtube_video_id": "9McJ3GobPaY",
              "start_seconds": 0,
              "end_seconds": 90,
              "capture_label": "I tried it"
            }
          ]
        }
      }
    ]
  }
EOF
