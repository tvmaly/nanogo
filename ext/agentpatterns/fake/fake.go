package fake

import (
	"github.com/tvmaly/nanogo/core/contracts"
	contractfake "github.com/tvmaly/nanogo/core/contracts/fake"
)

type Runtime = contractfake.PatternRuntime

var _ contracts.PatternRuntime = (*Runtime)(nil)
