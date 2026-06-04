package gateway_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/tvmaly/nanogo/modules/browser"
	"github.com/tvmaly/nanogo/modules/browser/fake"
	"github.com/tvmaly/nanogo/modules/gateway"
)

func TestBrowserGatewayOperationsHiddenWhenDisabled(t *testing.T) {
	svc := gateway.New(gateway.Config{})
	for _, method := range svc.Status().Methods {
		if strings.HasPrefix(method, "browser.") {
			t.Fatalf("unexpected browser method when disabled: %s", method)
		}
	}
}

func TestBrowserGatewayOperationsRegisteredWhenEnabled(t *testing.T) {
	browserSvc, err := browser.NewService(browser.ServiceConfig{Controller: fake.New()})
	if err != nil {
		t.Fatal(err)
	}
	svc := gateway.New(gateway.Config{Browser: browserSvc})
	methods := svc.Status().Methods
	for _, want := range []string{"browser.health", "browser.navigate", "browser.media.seek"} {
		if !containsMethod(methods, want) {
			t.Fatalf("missing %s in %v", want, methods)
		}
	}
	res, err := svc.Dispatch(context.Background(), gateway.Request{Method: "browser.health"})
	if err != nil {
		t.Fatalf("health dispatch: %v", err)
	}
	if health, ok := res.(browser.Health); !ok || !health.OK {
		t.Fatalf("unexpected health response: %#v", res)
	}
}

func TestBrowserGatewayNavigateUsesSharedService(t *testing.T) {
	browserSvc, err := browser.NewService(browser.ServiceConfig{Controller: fake.New(), Policy: browser.Policy{AllowedDomains: []string{"example.test"}}})
	if err != nil {
		t.Fatal(err)
	}
	svc := gateway.New(gateway.Config{Browser: browserSvc})
	start, err := browserSvc.Start(context.Background(), browser.StartRequest{SessionName: "gateway"})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(browser.NavigateRequest{SessionID: start.ID, URL: "https://blocked.test"})
	if _, err := svc.Dispatch(context.Background(), gateway.Request{Method: "browser.navigate", Params: raw}); err == nil {
		t.Fatal("expected shared browser service policy denial")
	}
}

func containsMethod(methods []string, want string) bool {
	for _, method := range methods {
		if method == want {
			return true
		}
	}
	return false
}
