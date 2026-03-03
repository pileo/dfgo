// Package cxdbstore records pipeline and agent events as typed CXDB turns.
package cxdbstore

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"dfgo/internal/agent/event"
	"dfgo/internal/attractor/events"

	cxdb "github.com/strongdm/ai-cxdb/clients/go"
	cxdbtypes "github.com/strongdm/ai-cxdb/clients/go/types"
)

// CXDBClient abstracts the CXDB client methods used by the recorder.
// The real *cxdb.Client satisfies this interface.
type CXDBClient interface {
	CreateContext(ctx context.Context, baseTurnID uint64) (*cxdb.ContextHead, error)
	ForkContext(ctx context.Context, baseTurnID uint64) (*cxdb.ContextHead, error)
	AppendTurn(ctx context.Context, req *cxdb.AppendRequest) (*cxdb.AppendResult, error)
	Close() error
}

// Config configures the CXDB recorder.
type Config struct {
	Address   string // CXDB binary protocol address (e.g., "localhost:9009")
	ClientTag string // Client identifier (e.g., "dfgo")
	RunID     string
	Pipeline  string
}

// Recorder appends typed turns to a CXDB context in response to pipeline and
// agent events. It is safe for concurrent use.
type Recorder struct {
	client    CXDBClient
	contextID uint64
	headTurn  uint64
	runID     string
	pipeline  string
	mu        sync.Mutex
}

// New connects to CXDB, creates a context, and returns a ready Recorder.
// Returns an error if CXDB is unreachable (fail-fast behavior).
func New(ctx context.Context, cfg Config) (*Recorder, error) {
	addr := cfg.Address
	// Strip scheme if someone passes a URL like "http://localhost:9009".
	// cxdb.Dial expects a raw host:port for TCP.
	addr = strings.TrimPrefix(addr, "http://")
	addr = strings.TrimPrefix(addr, "https://")

	client, err := cxdb.Dial(addr,
		cxdb.WithClientTag(cfg.ClientTag),
	)
	if err != nil {
		return nil, fmt.Errorf("cxdb unreachable at %s: %w", cfg.Address, err)
	}

	head, err := client.CreateContext(ctx, 0)
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("cxdb create context: %w", err)
	}

	// Publish type registry bundle (best-effort, via HTTP API).
	go PublishBundle(HTTPAddrFromBinary(addr))

	rec := &Recorder{
		client:    client,
		contextID: head.ContextID,
		headTurn:  head.HeadTurnID,
		runID:     cfg.RunID,
		pipeline:  cfg.Pipeline,
	}

	// Append provenance as the first turn (depth=1 convention).
	rec.appendProvenance(cfg.ClientTag, cfg.RunID, cfg.Pipeline)

	return rec, nil
}

// NewWithClient creates a Recorder using an existing CXDBClient.
// Used for testing with mock clients.
func NewWithClient(ctx context.Context, client CXDBClient, runID, pipeline string) (*Recorder, error) {
	head, err := client.CreateContext(ctx, 0)
	if err != nil {
		return nil, fmt.Errorf("cxdb create context: %w", err)
	}

	return &Recorder{
		client:    client,
		contextID: head.ContextID,
		headTurn:  head.HeadTurnID,
		runID:     runID,
		pipeline:  pipeline,
	}, nil
}

// ContextID returns the CXDB context ID for this recorder.
func (r *Recorder) ContextID() uint64 { return r.contextID }

// HeadTurn returns the current head turn ID.
func (r *Recorder) HeadTurn() uint64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.headTurn
}

// OnEvent implements events.Callback for pipeline events.
func (r *Recorder) OnEvent(evt events.Event) {
	turn, ok := eventToTurn(evt)
	if !ok {
		return
	}
	r.append(turn.typeID, turn.typeVersion, turn.payload)
}

// OnAgentEvent records agent-level events within a stage.
func (r *Recorder) OnAgentEvent(nodeID string, evt event.Event) {
	turn, ok := agentEventToTurn(nodeID, evt)
	if !ok {
		return
	}
	r.append(turn.typeID, turn.typeVersion, turn.payload)
}

// Fork creates a new Recorder branching from the current head.
// The forked recorder shares the underlying client connection.
// Used for parallel branches.
func (r *Recorder) Fork(ctx context.Context) (*Recorder, error) {
	r.mu.Lock()
	headTurn := r.headTurn
	r.mu.Unlock()

	fork, err := r.client.ForkContext(ctx, headTurn)
	if err != nil {
		return nil, fmt.Errorf("cxdb fork: %w", err)
	}

	return &Recorder{
		client:    r.client, // shared client
		contextID: fork.ContextID,
		headTurn:  fork.HeadTurnID,
		runID:     r.runID,
		pipeline:  r.pipeline,
	}, nil
}

// Close closes the underlying CXDB client connection.
// Only call on the root recorder, not on forked recorders (they share the client).
func (r *Recorder) Close() error {
	return r.client.Close()
}

// appendProvenance appends a ConversationItem with ContextMetadata + Provenance
// as the first turn of the context. This follows the CXDB convention for
// context identification and makes pipelines browseable in the CXDB UI.
func (r *Recorder) appendProvenance(clientTag, runID, pipeline string) {
	prov := cxdbtypes.CaptureProcessProvenance("dfgo", "",
		cxdbtypes.WithEnvVars(nil),
	)
	prov.CorrelationID = runID

	item := &cxdbtypes.ConversationItem{
		ItemType:  cxdbtypes.ItemTypeSystem,
		Status:    cxdbtypes.ItemStatusComplete,
		Timestamp: prov.CapturedAt,
		System: &cxdbtypes.SystemMessage{
			Kind:    cxdbtypes.SystemKindInfo,
			Title:   "Pipeline: " + pipeline,
			Content: "dfgo pipeline run " + runID,
		},
		ContextMetadata: &cxdbtypes.ContextMetadata{
			ClientTag: clientTag,
			Title:     pipeline + " — " + shortID(runID),
			Labels:    []string{"pipeline", pipeline},
			Custom: map[string]string{
				"run_id":   runID,
				"pipeline": pipeline,
			},
			Provenance: prov,
		},
	}

	r.append(
		cxdbtypes.TypeIDConversationItem,
		cxdbtypes.TypeVersionConversationItem,
		mustEncode(item),
	)
}

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

func mustEncode(v any) []byte {
	data, err := cxdb.EncodeMsgpack(v)
	if err != nil {
		slog.Error("cxdb encode provenance failed", "error", err)
		return nil
	}
	return data
}

// append is the internal write path. Errors are logged but not propagated
// to avoid blocking pipeline execution.
func (r *Recorder) append(typeID string, typeVersion uint32, payload []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()

	result, err := r.client.AppendTurn(context.Background(), &cxdb.AppendRequest{
		ContextID:   r.contextID,
		TypeID:      typeID,
		TypeVersion: typeVersion,
		Payload:     payload,
	})
	if err != nil {
		slog.Error("cxdb append failed", "type", typeID, "error", err)
		return
	}
	r.headTurn = result.TurnID
}
