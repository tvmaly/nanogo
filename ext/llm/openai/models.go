package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/tvmaly/nanogo/modules/gateway"
)

type ModelCatalog struct {
	cfg    Config
	client *http.Client
}

func NewModelCatalog(cfg Config) *ModelCatalog {
	return &ModelCatalog{cfg: cfg, client: &http.Client{}}
}

func (c *ModelCatalog) ListModels(ctx context.Context) ([]gateway.ModelInfo, error) {
	base := strings.TrimRight(c.cfg.BaseURL, "/")
	if base == "" {
		base = "https://openrouter.ai/api/v1"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/models", nil)
	if err != nil {
		return nil, err
	}
	if c.cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("openrouter models: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var wire struct {
		Data []struct {
			ID            string         `json:"id"`
			Name          string         `json:"name"`
			ContextLength int            `json:"context_length"`
			Pricing       map[string]any `json:"pricing"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &wire); err != nil {
		return nil, fmt.Errorf("openrouter models: decode: %w", err)
	}
	out := make([]gateway.ModelInfo, 0, len(wire.Data))
	for _, m := range wire.Data {
		out = append(out, gateway.ModelInfo{ID: m.ID, Name: m.Name, Context: m.ContextLength, Pricing: m.Pricing})
	}
	return out, nil
}
