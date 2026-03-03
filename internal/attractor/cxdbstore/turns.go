package cxdbstore

import (
	"log/slog"

	"dfgo/internal/agent/event"
	"dfgo/internal/attractor/events"

	cxdb "github.com/strongdm/ai-cxdb/clients/go"
)

// Turn type identifiers. Each maps to a CXDB type ID string.
const (
	TypePipelineStarted   = "com.dfgo.pipeline.started"
	TypePipelineCompleted = "com.dfgo.pipeline.completed"
	TypePipelineFailed    = "com.dfgo.pipeline.failed"
	TypeStageStarted      = "com.dfgo.stage.started"
	TypeStageCompleted    = "com.dfgo.stage.completed"
	TypeStageFailed       = "com.dfgo.stage.failed"
	TypeStageRetrying     = "com.dfgo.stage.retrying"
	TypeParallelStarted   = "com.dfgo.parallel.started"
	TypeParallelBranch    = "com.dfgo.parallel.branch"
	TypeParallelCompleted = "com.dfgo.parallel.completed"
	TypeInterviewStarted  = "com.dfgo.interview.started"
	TypeInterviewCompleted = "com.dfgo.interview.completed"
	TypeInterviewTimeout  = "com.dfgo.interview.timeout"
	TypeCheckpointSaved   = "com.dfgo.checkpoint.saved"
	TypeAgentTurnStart    = "com.dfgo.agent.turn.start"
	TypeAgentLLMResponse  = "com.dfgo.agent.llm.response"
	TypeAgentToolExec     = "com.dfgo.agent.tool.exec"
	TypeAgentLoopDetected = "com.dfgo.agent.loop.detected"
)

// All turn structs use msgpack numeric tags.
// Tags are never reused within a type (append-only schema evolution).

type PipelineStartedTurn struct {
	RunID     string `msgpack:"1"`
	Pipeline  string `msgpack:"2"`
	StartNode string `msgpack:"3"`
	Timestamp uint64 `msgpack:"4"` // unix_ms
}

type PipelineCompletedTurn struct {
	RunID     string `msgpack:"1"`
	Pipeline  string `msgpack:"2"`
	Status    string `msgpack:"3"`
	Timestamp uint64 `msgpack:"4"`
}

type PipelineFailedTurn struct {
	RunID     string `msgpack:"1"`
	Error     string `msgpack:"2"`
	Timestamp uint64 `msgpack:"3"`
}

type StageStartedTurn struct {
	NodeID    string `msgpack:"1"`
	NodeType  string `msgpack:"2"`
	Shape     string `msgpack:"3"`
	Timestamp uint64 `msgpack:"4"`
}

type StageCompletedTurn struct {
	NodeID    string `msgpack:"1"`
	Status    string `msgpack:"2"`
	Notes     string `msgpack:"3"`
	Timestamp uint64 `msgpack:"4"`
}

type StageFailedTurn struct {
	NodeID        string `msgpack:"1"`
	Status        string `msgpack:"2"`
	FailureReason string `msgpack:"3"`
	FailureClass  string `msgpack:"4"`
	Timestamp     uint64 `msgpack:"5"`
}

type StageRetryingTurn struct {
	NodeID    string `msgpack:"1"`
	Attempt   int    `msgpack:"2"`
	MaxRetry  int    `msgpack:"3"`
	Timestamp uint64 `msgpack:"4"`
}

type CheckpointSavedTurn struct {
	CurrentNode string `msgpack:"1"`
	CommitSHA   string `msgpack:"2"`
	Timestamp   uint64 `msgpack:"3"`
}

type ParallelStartedTurn struct {
	NodeID      string   `msgpack:"1"`
	BranchCount int      `msgpack:"2"`
	BranchIDs   []string `msgpack:"3"`
	JoinPolicy  string   `msgpack:"4"`
	Timestamp   uint64   `msgpack:"5"`
}

type ParallelBranchTurn struct {
	NodeID    string `msgpack:"1"`
	BranchKey string `msgpack:"2"`
	Event     string `msgpack:"3"`
	Status    string `msgpack:"4"`
	Timestamp uint64 `msgpack:"5"`
}

type ParallelCompletedTurn struct {
	NodeID     string `msgpack:"1"`
	WinnerKey  string `msgpack:"2"`
	JoinPolicy string `msgpack:"3"`
	Timestamp  uint64 `msgpack:"4"`
}

type InterviewTurn struct {
	NodeID    string `msgpack:"1"`
	Event     string `msgpack:"2"`
	Question  string `msgpack:"3"`
	Answer    string `msgpack:"4"`
	Timestamp uint64 `msgpack:"5"`
}

type AgentTurnStartTurn struct {
	NodeID    string `msgpack:"1"`
	Round     int    `msgpack:"2"`
	Timestamp uint64 `msgpack:"3"`
}

type AgentLLMResponseTurn struct {
	NodeID       string `msgpack:"1"`
	Model        string `msgpack:"2"`
	FinishReason string `msgpack:"3"`
	InputTokens  int    `msgpack:"4"`
	OutputTokens int    `msgpack:"5"`
	Timestamp    uint64 `msgpack:"6"`
}

type AgentToolExecTurn struct {
	NodeID    string `msgpack:"1"`
	ToolName  string `msgpack:"2"`
	CallID    string `msgpack:"3"`
	IsError   bool   `msgpack:"4"`
	Duration  uint64 `msgpack:"5"` // ms
	Timestamp uint64 `msgpack:"6"`
}

type AgentLoopDetectedTurn struct {
	NodeID    string `msgpack:"1"`
	ToolName  string `msgpack:"2"`
	Timestamp uint64 `msgpack:"3"`
}

// turnData holds the encoded turn ready for appending to CXDB.
type turnData struct {
	typeID      string
	typeVersion uint32
	payload     []byte
}

// eventToTurn maps a pipeline event to a typed CXDB turn.
func eventToTurn(evt events.Event) (turnData, bool) {
	ts := uint64(evt.Timestamp.UnixMilli())

	switch evt.Type {
	case events.PipelineStarted:
		return encode(TypePipelineStarted, 1, PipelineStartedTurn{
			RunID:     str(evt.Data, "run_id"),
			Pipeline:  str(evt.Data, "pipeline"),
			StartNode: str(evt.Data, "start"),
			Timestamp: ts,
		})
	case events.PipelineCompleted:
		return encode(TypePipelineCompleted, 1, PipelineCompletedTurn{
			RunID:     str(evt.Data, "run_id"),
			Pipeline:  str(evt.Data, "pipeline"),
			Status:    "completed",
			Timestamp: ts,
		})
	case events.PipelineFailed:
		return encode(TypePipelineFailed, 1, PipelineFailedTurn{
			RunID:     str(evt.Data, "run_id"),
			Error:     str(evt.Data, "error"),
			Timestamp: ts,
		})
	case events.StageStarted:
		return encode(TypeStageStarted, 1, StageStartedTurn{
			NodeID:    str(evt.Data, "node_id"),
			NodeType:  str(evt.Data, "type"),
			Shape:     str(evt.Data, "shape"),
			Timestamp: ts,
		})
	case events.StageCompleted:
		return encode(TypeStageCompleted, 1, StageCompletedTurn{
			NodeID:    str(evt.Data, "node_id"),
			Status:    str(evt.Data, "status"),
			Timestamp: ts,
		})
	case events.StageFailed:
		return encode(TypeStageFailed, 1, StageFailedTurn{
			NodeID:        str(evt.Data, "node_id"),
			Status:        str(evt.Data, "status"),
			FailureReason: str(evt.Data, "reason"),
			FailureClass:  str(evt.Data, "failure_class"),
			Timestamp:     ts,
		})
	case events.StageRetrying:
		return encode(TypeStageRetrying, 1, StageRetryingTurn{
			NodeID:    str(evt.Data, "node_id"),
			Attempt:   intVal(evt.Data, "attempt"),
			MaxRetry:  intVal(evt.Data, "max"),
			Timestamp: ts,
		})
	case events.CheckpointSaved:
		return encode(TypeCheckpointSaved, 1, CheckpointSavedTurn{
			CurrentNode: str(evt.Data, "current_node"),
			Timestamp:   ts,
		})
	case events.ParallelStarted:
		return encode(TypeParallelStarted, 1, ParallelStartedTurn{
			NodeID:      str(evt.Data, "node_id"),
			BranchCount: intVal(evt.Data, "branch_count"),
			JoinPolicy:  str(evt.Data, "join_policy"),
			Timestamp:   ts,
		})
	case events.ParallelBranchStarted:
		return encode(TypeParallelBranch, 1, ParallelBranchTurn{
			NodeID:    str(evt.Data, "node_id"),
			BranchKey: str(evt.Data, "branch_key"),
			Event:     "started",
			Timestamp: ts,
		})
	case events.ParallelBranchCompleted:
		return encode(TypeParallelBranch, 1, ParallelBranchTurn{
			NodeID:    str(evt.Data, "node_id"),
			BranchKey: str(evt.Data, "branch_key"),
			Event:     "completed",
			Status:    str(evt.Data, "status"),
			Timestamp: ts,
		})
	case events.ParallelCompleted:
		return encode(TypeParallelCompleted, 1, ParallelCompletedTurn{
			NodeID:     str(evt.Data, "node_id"),
			WinnerKey:  str(evt.Data, "winner"),
			JoinPolicy: str(evt.Data, "join_policy"),
			Timestamp:  ts,
		})
	case events.InterviewStarted:
		return encode(TypeInterviewStarted, 1, InterviewTurn{
			NodeID:    str(evt.Data, "node_id"),
			Event:     "started",
			Question:  str(evt.Data, "question"),
			Timestamp: ts,
		})
	case events.InterviewCompleted:
		return encode(TypeInterviewCompleted, 1, InterviewTurn{
			NodeID:    str(evt.Data, "node_id"),
			Event:     "completed",
			Answer:    str(evt.Data, "answer"),
			Timestamp: ts,
		})
	case events.InterviewTimeout:
		return encode(TypeInterviewTimeout, 1, InterviewTurn{
			NodeID:    str(evt.Data, "node_id"),
			Event:     "timeout",
			Timestamp: ts,
		})
	default:
		return turnData{}, false
	}
}

// agentEventToTurn maps an agent event to a typed CXDB turn.
func agentEventToTurn(nodeID string, evt event.Event) (turnData, bool) {
	ts := uint64(evt.Timestamp.UnixMilli())

	switch evt.Type {
	case event.TurnStart:
		return encode(TypeAgentTurnStart, 1, AgentTurnStartTurn{
			NodeID:    nodeID,
			Round:     intVal(evt.Data, "round"),
			Timestamp: ts,
		})
	case event.LLMResponse:
		return encode(TypeAgentLLMResponse, 1, AgentLLMResponseTurn{
			NodeID:       nodeID,
			Model:        str(evt.Data, "model"),
			FinishReason: str(evt.Data, "finish_reason"),
			InputTokens:  intVal(evt.Data, "input_tokens"),
			OutputTokens: intVal(evt.Data, "output_tokens"),
			Timestamp:    ts,
		})
	case event.ToolEnd:
		return encode(TypeAgentToolExec, 1, AgentToolExecTurn{
			NodeID:    nodeID,
			ToolName:  str(evt.Data, "tool"),
			CallID:    str(evt.Data, "call_id"),
			IsError:   boolVal(evt.Data, "is_error"),
			Timestamp: ts,
		})
	case event.LoopDetected:
		return encode(TypeAgentLoopDetected, 1, AgentLoopDetectedTurn{
			NodeID:    nodeID,
			ToolName:  str(evt.Data, "tool"),
			Timestamp: ts,
		})
	default:
		return turnData{}, false
	}
}

func encode(typeID string, version uint32, v any) (turnData, bool) {
	payload, err := cxdb.EncodeMsgpack(v)
	if err != nil {
		slog.Error("cxdb encode failed", "type", typeID, "error", err)
		return turnData{}, false
	}
	return turnData{typeID: typeID, typeVersion: version, payload: payload}, true
}

// Helper functions for extracting typed values from event data maps.

func str(data map[string]any, key string) string {
	if data == nil {
		return ""
	}
	v, ok := data[key]
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return s
}

func intVal(data map[string]any, key string) int {
	if data == nil {
		return 0
	}
	v, ok := data[key]
	if !ok {
		return 0
	}
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	default:
		return 0
	}
}

func boolVal(data map[string]any, key string) bool {
	if data == nil {
		return false
	}
	v, ok := data[key]
	if !ok {
		return false
	}
	b, _ := v.(bool)
	return b
}
