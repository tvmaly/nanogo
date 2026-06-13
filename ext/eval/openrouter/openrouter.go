package openrouter

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/tvmaly/nanogo/core/contracts"
)

type Config struct {
	Model string
}

type Observer struct {
	cfg Config
}

func New(cfg Config) Observer {
	if cfg.Model == "" {
		cfg.Model = os.Getenv("VISION_MODEL")
	}
	return Observer{cfg: cfg}
}

func (o Observer) BuildRequest(req contracts.ActivityObservationRequest) (map[string]any, error) {
	if o.cfg.Model == "" {
		return nil, fmt.Errorf("VISION_MODEL is required")
	}
	content := []map[string]any{{"type": "text", "text": "Return structured observations for the rubric checks. Do not decide mastery."}}
	for _, f := range req.FrameRefs {
		content = append(content, map[string]any{"type": "image_url", "image_url": map[string]any{"url": f.URI}})
	}
	return map[string]any{
		"model": o.cfg.Model,
		"messages": []map[string]any{{
			"role":    "user",
			"content": content,
		}},
		"response_format": map[string]any{"type": "json_object"},
	}, nil
}

func (o Observer) ObserveActivity(_ context.Context, req contracts.ActivityObservationRequest) (contracts.ActivityObservation, error) {
	shape, err := o.BuildRequest(req)
	if err != nil {
		return contracts.ActivityObservation{}, err
	}
	data, _ := json.Marshal(shape)
	return contracts.ActivityObservation{SchemaVersion: "activity.observation.v1", Observer: "openrouter", RawRef: string(data)}, nil
}
