# Phase 22 Frame Evaluation Spike

Schema: `phase22.frame_eval_spike.v1`

This spike records the decision thresholds for frame-based observation of short physical-skill attempts. Fixture clips and still frames are used for repository tests; real child media is optional local-only input for manual review.

## Decision

Default observer: `ext/eval/manual`.

OpenRouter vision remains experimental until a family runs the local fixture smoke with their chosen model and confirms the thresholds below.

Default fps recommendation: `8 fps`, capped at `24` frames per evaluation, downscaled to `720p`.

Default model id: use `VISION_MODEL` from the environment for live OpenRouter vision smoke tests.

## Thresholds

- Parent-agreement target: at least 80 percent agreement on required checks.
- False-positive guard: no critical required check may pass when the parent labels it absent.
- Cost guard: measured cost must be less than or equal to `eval.max_cost_usd_per_eval`.

## Fixture Result

Fixture: `testdata/phase22/frames_manifest.json`

Measured cost against `eval.max_cost_usd_per_eval`: local fixture and manual observer cost is `0.00`, which is below the default cap.

Fallback decision: if parent agreement, false-positive, or cost thresholds fail, use `ext/eval/manual` as the default observer and treat `ext/eval/openrouter` as experimental.

## Rubric Authoring Rules

- Prefer visually checkable end states over vague motion judgments.
- Mark critical checks explicitly.
- Keep each required check observable from sampled frames.
- Do not ask the model to decide mastery; request structured observations only.
