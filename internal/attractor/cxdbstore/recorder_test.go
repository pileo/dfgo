package cxdbstore

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"dfgo/internal/agent/event"
	"dfgo/internal/attractor/events"

	cxdb "github.com/strongdm/ai-cxdb/clients/go"
	cxdbtypes "github.com/strongdm/ai-cxdb/clients/go/types"
)

// mockClient implements CXDBClient for testing.
type mockClient struct {
	mu          sync.Mutex
	nextCtxID   uint64
	nextTurnID  uint64
	turns       []appendedTurn
	forks       []uint64 // baseTurnIDs passed to ForkContext
	closeCalled bool
}

type appendedTurn struct {
	contextID   uint64
	typeID      string
	typeVersion uint32
	payload     []byte
}

func newMockClient() *mockClient {
	return &mockClient{nextCtxID: 100, nextTurnID: 1000}
}

func (m *mockClient) CreateContext(_ context.Context, baseTurnID uint64) (*cxdb.ContextHead, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nextCtxID++
	return &cxdb.ContextHead{
		ContextID:  m.nextCtxID,
		HeadTurnID: 0,
	}, nil
}

func (m *mockClient) ForkContext(_ context.Context, baseTurnID uint64) (*cxdb.ContextHead, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.forks = append(m.forks, baseTurnID)
	m.nextCtxID++
	return &cxdb.ContextHead{
		ContextID:  m.nextCtxID,
		HeadTurnID: baseTurnID,
	}, nil
}

func (m *mockClient) AppendTurn(_ context.Context, req *cxdb.AppendRequest) (*cxdb.AppendResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nextTurnID++
	m.turns = append(m.turns, appendedTurn{
		contextID:   req.ContextID,
		typeID:      req.TypeID,
		typeVersion: req.TypeVersion,
		payload:     req.Payload,
	})
	return &cxdb.AppendResult{
		ContextID: req.ContextID,
		TurnID:    m.nextTurnID,
	}, nil
}

func (m *mockClient) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closeCalled = true
	return nil
}

func (m *mockClient) getTurns() []appendedTurn {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]appendedTurn, len(m.turns))
	copy(cp, m.turns)
	return cp
}

func TestNewRecorder(t *testing.T) {
	mc := newMockClient()
	rec, err := NewWithClient(context.Background(), mc, "run-1", "test-pipeline")
	if err != nil {
		t.Fatal(err)
	}
	defer rec.Close()

	if rec.ContextID() == 0 {
		t.Error("expected non-zero context ID")
	}
}

func TestAppendProvenance(t *testing.T) {
	mc := newMockClient()
	rec, err := NewWithClient(context.Background(), mc, "a828f97b-de97-4bac-8953-e29db603975f", "my-pipeline")
	if err != nil {
		t.Fatal(err)
	}

	rec.appendProvenance("dfgo", "a828f97b-de97-4bac-8953-e29db603975f", "my-pipeline")

	turns := mc.getTurns()
	if len(turns) != 1 {
		t.Fatalf("expected 1 provenance turn, got %d", len(turns))
	}
	if turns[0].typeID != "cxdb.ConversationItem" {
		t.Errorf("expected type cxdb.ConversationItem, got %q", turns[0].typeID)
	}
	if turns[0].typeVersion != 3 {
		t.Errorf("expected version 3, got %d", turns[0].typeVersion)
	}
	if len(turns[0].payload) == 0 {
		t.Error("provenance payload is empty")
	}

	// Decode and verify key fields.
	var item cxdbtypes.ConversationItem
	if err := cxdb.DecodeMsgpackInto(turns[0].payload, &item); err != nil {
		t.Fatalf("decode provenance: %v", err)
	}
	if item.ItemType != cxdbtypes.ItemTypeSystem {
		t.Errorf("expected item_type system, got %q", item.ItemType)
	}
	if item.ContextMetadata == nil {
		t.Fatal("context_metadata is nil")
	}
	if item.ContextMetadata.ClientTag != "dfgo" {
		t.Errorf("expected client_tag dfgo, got %q", item.ContextMetadata.ClientTag)
	}
	if item.ContextMetadata.Provenance == nil {
		t.Fatal("provenance is nil")
	}
	if item.ContextMetadata.Provenance.ServiceName != "dfgo" {
		t.Errorf("expected service_name dfgo, got %q", item.ContextMetadata.Provenance.ServiceName)
	}
	if item.ContextMetadata.Provenance.CorrelationID != "a828f97b-de97-4bac-8953-e29db603975f" {
		t.Errorf("expected correlation_id to be run_id, got %q", item.ContextMetadata.Provenance.CorrelationID)
	}
	if item.ContextMetadata.Provenance.ProcessPID == 0 {
		t.Error("expected non-zero process PID")
	}
	if item.ContextMetadata.Title != "my-pipeline — a828f97b" {
		t.Errorf("expected title with short run ID, got %q", item.ContextMetadata.Title)
	}
}

func TestOnEvent_PipelineStarted(t *testing.T) {
	mc := newMockClient()
	rec, err := NewWithClient(context.Background(), mc, "run-1", "test-pipeline")
	if err != nil {
		t.Fatal(err)
	}

	rec.OnEvent(events.Event{
		Type:      events.PipelineStarted,
		Timestamp: time.Unix(1700000000, 0),
		Data: map[string]any{
			"run_id":   "run-1",
			"pipeline": "test-pipeline",
			"start":    "start_node",
		},
	})

	turns := mc.getTurns()
	if len(turns) != 1 {
		t.Fatalf("expected 1 turn, got %d", len(turns))
	}
	if turns[0].typeID != TypePipelineStarted {
		t.Errorf("expected type %q, got %q", TypePipelineStarted, turns[0].typeID)
	}
	if turns[0].typeVersion != 1 {
		t.Errorf("expected version 1, got %d", turns[0].typeVersion)
	}
}

func TestOnEvent_AllPipelineEvents(t *testing.T) {
	eventTypes := []struct {
		evtType  events.Type
		turnType string
		data     map[string]any
	}{
		{events.PipelineStarted, TypePipelineStarted, map[string]any{"run_id": "r", "pipeline": "p", "start": "s"}},
		{events.PipelineCompleted, TypePipelineCompleted, map[string]any{"run_id": "r", "pipeline": "p"}},
		{events.PipelineFailed, TypePipelineFailed, map[string]any{"run_id": "r", "error": "oops"}},
		{events.StageStarted, TypeStageStarted, map[string]any{"node_id": "n", "type": "t", "shape": "box"}},
		{events.StageCompleted, TypeStageCompleted, map[string]any{"node_id": "n", "status": "success"}},
		{events.StageFailed, TypeStageFailed, map[string]any{"node_id": "n", "status": "fail", "reason": "err"}},
		{events.StageRetrying, TypeStageRetrying, map[string]any{"node_id": "n", "attempt": 2, "max": 5}},
		{events.CheckpointSaved, TypeCheckpointSaved, map[string]any{"current_node": "n"}},
		{events.ParallelStarted, TypeParallelStarted, map[string]any{"node_id": "n", "branch_count": 3}},
		{events.ParallelBranchStarted, TypeParallelBranch, map[string]any{"node_id": "n", "branch_key": "b1"}},
		{events.ParallelBranchCompleted, TypeParallelBranch, map[string]any{"node_id": "n", "branch_key": "b1", "status": "success"}},
		{events.ParallelCompleted, TypeParallelCompleted, map[string]any{"node_id": "n", "winner": "b1"}},
		{events.InterviewStarted, TypeInterviewStarted, map[string]any{"node_id": "n", "question": "q?"}},
		{events.InterviewCompleted, TypeInterviewCompleted, map[string]any{"node_id": "n", "answer": "a"}},
		{events.InterviewTimeout, TypeInterviewTimeout, map[string]any{"node_id": "n"}},
	}

	mc := newMockClient()
	rec, err := NewWithClient(context.Background(), mc, "run-1", "test")
	if err != nil {
		t.Fatal(err)
	}

	for _, et := range eventTypes {
		rec.OnEvent(events.Event{
			Type:      et.evtType,
			Timestamp: time.Now(),
			Data:      et.data,
		})
	}

	turns := mc.getTurns()
	if len(turns) != len(eventTypes) {
		t.Fatalf("expected %d turns, got %d", len(eventTypes), len(turns))
	}

	for i, et := range eventTypes {
		if turns[i].typeID != et.turnType {
			t.Errorf("turn %d: expected type %q, got %q", i, et.turnType, turns[i].typeID)
		}
	}
}

func TestOnEvent_UnknownEventIgnored(t *testing.T) {
	mc := newMockClient()
	rec, err := NewWithClient(context.Background(), mc, "run-1", "test")
	if err != nil {
		t.Fatal(err)
	}

	rec.OnEvent(events.Event{
		Type:      "unknown.event",
		Timestamp: time.Now(),
		Data:      map[string]any{},
	})

	turns := mc.getTurns()
	if len(turns) != 0 {
		t.Fatalf("expected 0 turns for unknown event, got %d", len(turns))
	}
}

func TestOnAgentEvent(t *testing.T) {
	agentEvents := []struct {
		evtType  event.Type
		turnType string
		data     map[string]any
	}{
		{event.TurnStart, TypeAgentTurnStart, map[string]any{"round": 1}},
		{event.LLMResponse, TypeAgentLLMResponse, map[string]any{"model": "claude", "finish_reason": "stop", "input_tokens": 100, "output_tokens": 50}},
		{event.ToolEnd, TypeAgentToolExec, map[string]any{"tool": "read", "call_id": "c1", "is_error": false}},
		{event.LoopDetected, TypeAgentLoopDetected, map[string]any{"tool": "write"}},
	}

	mc := newMockClient()
	rec, err := NewWithClient(context.Background(), mc, "run-1", "test")
	if err != nil {
		t.Fatal(err)
	}

	for _, ae := range agentEvents {
		rec.OnAgentEvent("test_node", event.Event{
			Type:      ae.evtType,
			Timestamp: time.Now(),
			Data:      ae.data,
		})
	}

	turns := mc.getTurns()
	if len(turns) != len(agentEvents) {
		t.Fatalf("expected %d turns, got %d", len(agentEvents), len(turns))
	}

	for i, ae := range agentEvents {
		if turns[i].typeID != ae.turnType {
			t.Errorf("turn %d: expected type %q, got %q", i, ae.turnType, turns[i].typeID)
		}
	}
}

func TestOnAgentEvent_UnknownIgnored(t *testing.T) {
	mc := newMockClient()
	rec, err := NewWithClient(context.Background(), mc, "run-1", "test")
	if err != nil {
		t.Fatal(err)
	}

	rec.OnAgentEvent("node", event.Event{
		Type:      event.LLMStreamStart, // not mapped
		Timestamp: time.Now(),
		Data:      map[string]any{},
	})

	if len(mc.getTurns()) != 0 {
		t.Fatal("expected unmapped agent event to be ignored")
	}
}

func TestFork(t *testing.T) {
	mc := newMockClient()
	rec, err := NewWithClient(context.Background(), mc, "run-1", "test")
	if err != nil {
		t.Fatal(err)
	}

	// Append a turn to advance the head.
	rec.OnEvent(events.Event{
		Type:      events.PipelineStarted,
		Timestamp: time.Now(),
		Data:      map[string]any{"run_id": "r", "pipeline": "p", "start": "s"},
	})

	forked, err := rec.Fork(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if forked.ContextID() == rec.ContextID() {
		t.Error("forked recorder should have a different context ID")
	}
	if forked.ContextID() == 0 {
		t.Error("forked recorder should have non-zero context ID")
	}

	// Verify fork was called on the mock.
	mc.mu.Lock()
	if len(mc.forks) != 1 {
		t.Errorf("expected 1 fork call, got %d", len(mc.forks))
	}
	mc.mu.Unlock()
}

func TestEncodeDecodeRoundTrip(t *testing.T) {
	// Verify encode/decode round-trip for representative turn types.
	t.Run("PipelineStarted", func(t *testing.T) {
		original := PipelineStartedTurn{RunID: "r", Pipeline: "p", StartNode: "s", Timestamp: 1700000000000}
		encoded, err := cxdb.EncodeMsgpack(original)
		if err != nil {
			t.Fatalf("encode: %v", err)
		}
		var decoded PipelineStartedTurn
		if err := cxdb.DecodeMsgpackInto(encoded, &decoded); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if decoded.RunID != original.RunID || decoded.Pipeline != original.Pipeline || decoded.Timestamp != original.Timestamp {
			t.Errorf("round-trip mismatch: got %+v, want %+v", decoded, original)
		}
	})

	t.Run("StageFailedAllFields", func(t *testing.T) {
		original := StageFailedTurn{NodeID: "n", Status: "fail", FailureReason: "err", FailureClass: "transient", Timestamp: 1700000000000}
		encoded, err := cxdb.EncodeMsgpack(original)
		if err != nil {
			t.Fatalf("encode: %v", err)
		}
		var decoded StageFailedTurn
		if err := cxdb.DecodeMsgpackInto(encoded, &decoded); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if decoded != original {
			t.Errorf("round-trip mismatch: got %+v, want %+v", decoded, original)
		}
	})

	t.Run("AgentLLMResponse", func(t *testing.T) {
		original := AgentLLMResponseTurn{NodeID: "n", Model: "claude", FinishReason: "stop", InputTokens: 100, OutputTokens: 50, Timestamp: 1700000000000}
		encoded, err := cxdb.EncodeMsgpack(original)
		if err != nil {
			t.Fatalf("encode: %v", err)
		}
		var decoded AgentLLMResponseTurn
		if err := cxdb.DecodeMsgpackInto(encoded, &decoded); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if decoded != original {
			t.Errorf("round-trip mismatch: got %+v, want %+v", decoded, original)
		}
	})

	t.Run("AgentToolExec", func(t *testing.T) {
		original := AgentToolExecTurn{NodeID: "n", ToolName: "read", CallID: "c1", IsError: true, Duration: 150, Timestamp: 1700000000000}
		encoded, err := cxdb.EncodeMsgpack(original)
		if err != nil {
			t.Fatalf("encode: %v", err)
		}
		var decoded AgentToolExecTurn
		if err := cxdb.DecodeMsgpackInto(encoded, &decoded); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if decoded != original {
			t.Errorf("round-trip mismatch: got %+v, want %+v", decoded, original)
		}
	})

	// Verify all turn types at least encode successfully.
	allTurns := []any{
		PipelineStartedTurn{}, PipelineCompletedTurn{}, PipelineFailedTurn{},
		StageStartedTurn{}, StageCompletedTurn{}, StageFailedTurn{}, StageRetryingTurn{},
		CheckpointSavedTurn{},
		ParallelStartedTurn{}, ParallelBranchTurn{}, ParallelCompletedTurn{},
		InterviewTurn{},
		AgentTurnStartTurn{}, AgentLLMResponseTurn{}, AgentToolExecTurn{}, AgentLoopDetectedTurn{},
	}
	for _, turn := range allTurns {
		encoded, err := cxdb.EncodeMsgpack(turn)
		if err != nil {
			t.Errorf("encode %T: %v", turn, err)
		}
		if len(encoded) == 0 {
			t.Errorf("encode %T: empty payload", turn)
		}
	}
}

func TestHeadTurnAdvances(t *testing.T) {
	mc := newMockClient()
	rec, err := NewWithClient(context.Background(), mc, "run-1", "test")
	if err != nil {
		t.Fatal(err)
	}

	initial := rec.HeadTurn()

	rec.OnEvent(events.Event{
		Type:      events.PipelineStarted,
		Timestamp: time.Now(),
		Data:      map[string]any{"run_id": "r", "pipeline": "p", "start": "s"},
	})

	after := rec.HeadTurn()
	if after == initial {
		t.Error("head turn should advance after append")
	}

	rec.OnEvent(events.Event{
		Type:      events.StageStarted,
		Timestamp: time.Now(),
		Data:      map[string]any{"node_id": "n"},
	})

	after2 := rec.HeadTurn()
	if after2 == after {
		t.Error("head turn should advance again after second append")
	}
}

func TestRegistryBundle(t *testing.T) {
	bundle := RegistryBundle()

	if bundle["bundle_id"] != BundleID {
		t.Errorf("expected bundle_id %q, got %q", BundleID, bundle["bundle_id"])
	}

	types, ok := bundle["types"].(map[string]any)
	if !ok {
		t.Fatal("types field missing or wrong type")
	}

	expectedTypes := []string{
		TypePipelineStarted, TypePipelineCompleted, TypePipelineFailed,
		TypeStageStarted, TypeStageCompleted, TypeStageFailed, TypeStageRetrying,
		TypeCheckpointSaved,
		TypeParallelStarted, TypeParallelBranch, TypeParallelCompleted,
		TypeInterviewStarted, TypeInterviewCompleted, TypeInterviewTimeout,
		TypeAgentTurnStart, TypeAgentLLMResponse, TypeAgentToolExec, TypeAgentLoopDetected,
	}

	for _, et := range expectedTypes {
		td, ok := types[et]
		if !ok {
			t.Errorf("missing type %q in registry bundle", et)
			continue
		}
		// Verify CXDB versioned format: {"versions": {"1": {"fields": {...}}}}
		tdMap, ok := td.(map[string]any)
		if !ok {
			t.Errorf("type %q: expected map, got %T", et, td)
			continue
		}
		versions, ok := tdMap["versions"].(map[string]any)
		if !ok {
			t.Errorf("type %q: missing 'versions' key", et)
			continue
		}
		v1, ok := versions["1"].(map[string]any)
		if !ok {
			t.Errorf("type %q: missing version '1'", et)
			continue
		}
		if _, ok := v1["fields"]; !ok {
			t.Errorf("type %q: version 1 missing 'fields'", et)
		}
	}

	// Verify bundle serializes to valid JSON (required for HTTP PUT).
	data, err := json.Marshal(bundle)
	if err != nil {
		t.Fatalf("bundle JSON marshal failed: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("bundle serialized to empty JSON")
	}
}

func TestHTTPAddrFromBinary(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"localhost:9009", "http://localhost:9010"},
		{"127.0.0.1:9009", "http://127.0.0.1:9010"},
		{"myhost:5000", "http://myhost:5001"},
		{"bad-addr", "http://localhost:9010"}, // fallback
	}
	for _, tt := range tests {
		got := HTTPAddrFromBinary(tt.input)
		if got != tt.want {
			t.Errorf("HTTPAddrFromBinary(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// Test helper functions
func TestStr(t *testing.T) {
	data := map[string]any{"key": "value", "num": 42}
	if got := str(data, "key"); got != "value" {
		t.Errorf("str: expected %q, got %q", "value", got)
	}
	if got := str(data, "missing"); got != "" {
		t.Errorf("str missing: expected empty, got %q", got)
	}
	if got := str(data, "num"); got != "" {
		t.Errorf("str non-string: expected empty, got %q", got)
	}
	if got := str(nil, "key"); got != "" {
		t.Errorf("str nil: expected empty, got %q", got)
	}
}

func TestIntVal(t *testing.T) {
	data := map[string]any{"i": 42, "i64": int64(100), "f": 3.14, "s": "nope"}
	if got := intVal(data, "i"); got != 42 {
		t.Errorf("intVal int: expected 42, got %d", got)
	}
	if got := intVal(data, "i64"); got != 100 {
		t.Errorf("intVal int64: expected 100, got %d", got)
	}
	if got := intVal(data, "f"); got != 3 {
		t.Errorf("intVal float64: expected 3, got %d", got)
	}
	if got := intVal(data, "s"); got != 0 {
		t.Errorf("intVal string: expected 0, got %d", got)
	}
	if got := intVal(data, "missing"); got != 0 {
		t.Errorf("intVal missing: expected 0, got %d", got)
	}
}

func TestBoolVal(t *testing.T) {
	data := map[string]any{"b": true, "s": "nope"}
	if got := boolVal(data, "b"); got != true {
		t.Errorf("boolVal true: expected true, got %v", got)
	}
	if got := boolVal(data, "s"); got != false {
		t.Errorf("boolVal string: expected false, got %v", got)
	}
	if got := boolVal(data, "missing"); got != false {
		t.Errorf("boolVal missing: expected false, got %v", got)
	}
}
