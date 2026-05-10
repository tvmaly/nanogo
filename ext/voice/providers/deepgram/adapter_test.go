package deepgram

import "testing"

func TestDeepgramDeferred(t *testing.T) {
	if Endpoint == "" {
		t.Fatal("missing endpoint")
	}
	if err := Deferred(); err == nil {
		t.Fatal("expected deferred error")
	}
}
