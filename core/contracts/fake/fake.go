package fake

import (
	"context"
	"sync"

	"github.com/tvmaly/nanogo/core/contracts"
)

var _ contracts.AgentRunner = (*AgentRunner)(nil)
var _ contracts.SubagentSpawner = (*SubagentSpawner)(nil)
var _ contracts.ToolCatalog = (*ToolRuntime)(nil)
var _ contracts.ToolInvoker = (*ToolRuntime)(nil)
var _ contracts.ToolRuntime = (*ToolRuntime)(nil)
var _ contracts.PatternRunner = (*PatternRuntime)(nil)
var _ contracts.PatternResumer = (*PatternRuntime)(nil)
var _ contracts.HandoffTarget = (*PatternRuntime)(nil)
var _ contracts.PatternRuntime = (*PatternRuntime)(nil)
var _ contracts.TraceSink = (*TraceSink)(nil)
var _ contracts.ApprovalGate = (*ApprovalGate)(nil)
var _ contracts.SpeechToText = (*SpeechToText)(nil)
var _ contracts.TextToSpeech = (*TextToSpeech)(nil)
var _ contracts.ActivityObserver = (*ActivityObserver)(nil)

type AgentRunner struct {
	Requests []contracts.AgentRequest
	Result   contracts.AgentResult
	Err      error
}

func (r *AgentRunner) RunAgent(_ context.Context, req contracts.AgentRequest) (contracts.AgentResult, error) {
	r.Requests = append(r.Requests, req)
	return r.Result, r.Err
}

type SubagentSpawner struct {
	Requests []contracts.SubagentRequest
	Result   contracts.SubagentResult
	Err      error
}

func (s *SubagentSpawner) SpawnSubagent(_ context.Context, req contracts.SubagentRequest) (contracts.SubagentResult, error) {
	s.Requests = append(s.Requests, req)
	return s.Result, s.Err
}

type ToolRuntime struct {
	Specs       []contracts.ToolSpec
	Invocations []contracts.ToolInvocation
	Result      contracts.ToolResult
	ListErr     error
	InvokeErr   error
}

func (r *ToolRuntime) ListTools(context.Context) ([]contracts.ToolSpec, error) {
	return append([]contracts.ToolSpec(nil), r.Specs...), r.ListErr
}

func (r *ToolRuntime) InvokeTool(_ context.Context, req contracts.ToolInvocation) (contracts.ToolResult, error) {
	r.Invocations = append(r.Invocations, req)
	return r.Result, r.InvokeErr
}

type PatternRuntime struct {
	RunRequests    []contracts.PatternRequest
	ResumeRequests []ResumeRequest
	HandoffInputs  []contracts.HandoffInput
	RunResult      contracts.PatternResult
	ResumeResult   contracts.PatternResult
	HandoffResult  contracts.HandoffResult
	RunErr         error
	ResumeErr      error
	HandoffErr     error
}

type ResumeRequest struct {
	CheckpointID string
	Input        contracts.ResumeInput
}

func (r *PatternRuntime) RunPattern(_ context.Context, req contracts.PatternRequest) (contracts.PatternResult, error) {
	r.RunRequests = append(r.RunRequests, req)
	return r.RunResult, r.RunErr
}

func (r *PatternRuntime) ResumePattern(_ context.Context, checkpointID string, input contracts.ResumeInput) (contracts.PatternResult, error) {
	r.ResumeRequests = append(r.ResumeRequests, ResumeRequest{CheckpointID: checkpointID, Input: input})
	return r.ResumeResult, r.ResumeErr
}

func (r *PatternRuntime) Handoff(_ context.Context, input contracts.HandoffInput) (contracts.HandoffResult, error) {
	r.HandoffInputs = append(r.HandoffInputs, input)
	return r.HandoffResult, r.HandoffErr
}

type TraceSink struct {
	Events []contracts.TraceEvent
	Err    error
}

func (s *TraceSink) EmitTrace(_ context.Context, event contracts.TraceEvent) error {
	s.Events = append(s.Events, event)
	return s.Err
}

type ApprovalGate struct {
	Requests []contracts.ApprovalRequest
	Result   contracts.ApprovalResult
	Err      error
}

type ActivityObserver struct {
	Requests []contracts.ActivityObservationRequest
	Result   contracts.ActivityObservation
	Err      error
}

func (o *ActivityObserver) ObserveActivity(_ context.Context, req contracts.ActivityObservationRequest) (contracts.ActivityObservation, error) {
	o.Requests = append(o.Requests, req)
	return o.Result, o.Err
}

func (g *ApprovalGate) RequestApproval(_ context.Context, req contracts.ApprovalRequest) (contracts.ApprovalResult, error) {
	g.Requests = append(g.Requests, req)
	return g.Result, g.Err
}

type SpeechToText struct {
	CapabilitiesResult contracts.STTCapabilities
	OpenErr            error
	Sessions           []*STTSession
	events             []contracts.TranscriptEvent
}

func NewSpeechToText(events ...contracts.TranscriptEvent) *SpeechToText {
	return &SpeechToText{
		CapabilitiesResult: contracts.STTCapabilities{
			Provider:        "fake",
			Local:           true,
			Streaming:       true,
			OfflineFiles:    true,
			SupportsPartial: true,
			SupportsTiming:  true,
		},
		events: append([]contracts.TranscriptEvent(nil), events...),
	}
}

func (s *SpeechToText) Capabilities(context.Context) (contracts.STTCapabilities, error) {
	return s.CapabilitiesResult, nil
}

func (s *SpeechToText) OpenSTTSession(_ context.Context, opts contracts.STTOptions) (contracts.STTSession, error) {
	if s.OpenErr != nil {
		return nil, s.OpenErr
	}
	sess := &STTSession{
		SessionID: opts.SessionID,
		events:    append([]contracts.TranscriptEvent(nil), s.events...),
		out:       make(chan contracts.TranscriptEvent, len(s.events)),
	}
	s.Sessions = append(s.Sessions, sess)
	return sess, nil
}

type STTSession struct {
	SessionID string
	Frames    []contracts.AudioFrame
	CloseErr  error

	mu     sync.Mutex
	events []contracts.TranscriptEvent
	out    chan contracts.TranscriptEvent
	next   int
	closed bool
}

func (s *STTSession) WriteAudio(_ context.Context, frame contracts.AudioFrame) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Frames = append(s.Frames, frame)
	if s.next < len(s.events) {
		event := s.events[s.next]
		if event.SessionID == "" {
			event.SessionID = s.SessionID
		}
		if event.Sequence == 0 {
			event.Sequence = int64(s.next + 1)
		}
		s.out <- event
		s.next++
	}
	return nil
}

func (s *STTSession) Events() <-chan contracts.TranscriptEvent { return s.out }

func (s *STTSession) Close(context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return s.CloseErr
	}
	s.closed = true
	close(s.out)
	return s.CloseErr
}

type TextToSpeech struct {
	CapabilitiesResult contracts.TTSCapabilities
	Requests           []contracts.SynthesisRequest
	Streams            []*TTSStream
	SynthesizeErr      error
}

func NewTextToSpeech() *TextToSpeech {
	return &TextToSpeech{
		CapabilitiesResult: contracts.TTSCapabilities{
			Provider:  "fake",
			Local:     true,
			Streaming: true,
			OutputFormats: []contracts.AudioFormat{{
				Encoding:     contracts.AudioEncodingPCM16,
				SampleRateHz: 24000,
				Channels:     1,
			}},
		},
	}
}

func (t *TextToSpeech) Capabilities(context.Context) (contracts.TTSCapabilities, error) {
	return t.CapabilitiesResult, nil
}

func (t *TextToSpeech) Synthesize(_ context.Context, req contracts.SynthesisRequest) (contracts.TTSStream, error) {
	if t.SynthesizeErr != nil {
		return nil, t.SynthesizeErr
	}
	t.Requests = append(t.Requests, req)
	stream := &TTSStream{events: make(chan contracts.TTSEvent, 3)}
	format := req.Options.OutputFormat
	if format.Encoding == "" {
		format = contracts.AudioFormat{Encoding: contracts.AudioEncodingPCM16, SampleRateHz: 24000, Channels: 1}
	}
	stream.events <- contracts.TTSEvent{SessionID: req.SessionID, Kind: contracts.TTSEventStarted, Format: format, Sequence: 1}
	stream.events <- contracts.TTSEvent{SessionID: req.SessionID, Kind: contracts.TTSEventAudio, Format: format, PCM: []byte(req.Text), Sequence: 2}
	stream.events <- contracts.TTSEvent{SessionID: req.SessionID, Kind: contracts.TTSEventDone, Format: format, Sequence: 3}
	t.Streams = append(t.Streams, stream)
	return stream, nil
}

type TTSStream struct {
	events chan contracts.TTSEvent
	Closed bool
}

func (s *TTSStream) Events() <-chan contracts.TTSEvent { return s.events }

func (s *TTSStream) Close(context.Context) error {
	if !s.Closed {
		s.Closed = true
		close(s.events)
	}
	return nil
}
