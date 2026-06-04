package fake_test

import (
	"testing"

	"github.com/tvmaly/nanogo/modules/browser"
	"github.com/tvmaly/nanogo/modules/browser/contracttest"
	"github.com/tvmaly/nanogo/modules/browser/fake"
)

func TestControllerContract(t *testing.T) {
	contracttest.RunControllerContract(t, func() browser.Controller {
		return fake.New()
	})
}
