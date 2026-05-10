package deepgram

import "fmt"

const Endpoint = "wss://agent.deepgram.com/v1/agent/converse"

func Deferred() error {
	return fmt.Errorf("deepgram voice provider is planned but deferred for Phase 15")
}
