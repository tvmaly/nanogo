package help

import (
	"context"
	"sort"
)

type MemoryCatalog struct {
	pack   Pack
	topics map[string]Topic
}

func NewCatalog(pack Pack) (*MemoryCatalog, error) {
	if errs := ValidatePack(pack); len(errs) > 0 {
		return nil, validationError(errs)
	}
	topics := make(map[string]Topic, len(pack.Topics))
	for _, t := range pack.Topics {
		topics[t.ID] = t
	}
	return &MemoryCatalog{pack: pack, topics: topics}, nil
}

func (c *MemoryCatalog) ListTopics(context.Context) ([]TopicMeta, error) {
	out := make([]TopicMeta, 0, len(c.topics))
	for _, t := range c.topics {
		out = append(out, t.TopicMeta)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (c *MemoryCatalog) GetTopic(_ context.Context, id string) (Topic, error) {
	t, ok := c.topics[id]
	if !ok {
		return Topic{}, NotFoundError{ID: id}
	}
	return t, nil
}

func (c *MemoryCatalog) RootTopics() []string {
	return append([]string(nil), c.pack.RootTopics...)
}
