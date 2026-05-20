package adaptive

import "github.com/tvmaly/nanogo/core/contracts"

type Evidence struct {
	Records []contracts.EvidenceRecord
}

func FromPatternResult(result contracts.PatternResult) Evidence {
	return Evidence{Records: append([]contracts.EvidenceRecord(nil), result.Evidence...)}
}
