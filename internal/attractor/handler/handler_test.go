package handler

import (
	"context"
	"testing"

	"dfgo/internal/attractor/interviewer"
	"dfgo/internal/attractor/model"
	"dfgo/internal/attractor/runtime"
	"dfgo/internal/llm"
)

// mockLLMProvider implements llm.ProviderAdapter for handler tests.
type mockLLMProvider struct {
	response string
}

func (m *mockLLMProvider) Name() string { return "mock" }
func (m *mockLLMProvider) Complete(_ context.Context, _ llm.Request) (*llm.Response, error) {
	return &llm.Response{
		Message:      llm.TextMessage(llm.RoleAssistant, m.response),
		FinishReason: llm.FinishStop,
	}, nil
}

func newMockClient(prov llm.ProviderAdapter) *llm.Client {
	return llm.NewClient(llm.WithProvider(prov))
}

func TestStartHandler(t *testing.T) {
	h := &StartHandler{}
	o, err := h.Execute(context.Background(), &model.Node{ID: "start"}, runtime.NewContext(), model.NewGraph("test"), "")
	if err != nil || o.Status != runtime.StatusSuccess {
		t.Fatal("start handler should succeed")
	}
}

func TestExitHandler(t *testing.T) {
	h := &ExitHandler{}
	o, err := h.Execute(context.Background(), &model.Node{ID: "exit"}, runtime.NewContext(), model.NewGraph("test"), "")
	if err != nil || o.Status != runtime.StatusSuccess {
		t.Fatal("exit handler should succeed")
	}
}

func TestConditionalHandler(t *testing.T) {
	h := &ConditionalHandler{}
	o, err := h.Execute(context.Background(), &model.Node{ID: "check"}, runtime.NewContext(), model.NewGraph("test"), "")
	if err != nil || o.Status != runtime.StatusSuccess {
		t.Fatal("conditional handler should succeed")
	}
}

func TestCodergenHandlerStub(t *testing.T) {
	h := NewCodergenHandler(nil)
	node := &model.Node{ID: "gen", Attrs: map[string]string{"prompt": "write code"}}
	o, err := h.Execute(context.Background(), node, runtime.NewContext(), model.NewGraph("test"), "")
	if err != nil {
		t.Fatal(err)
	}
	if o.Status != runtime.StatusSuccess {
		t.Fatalf("expected success, got %s", o.Status)
	}
	if o.ContextUpdates["gen.response"] == "" {
		t.Fatal("expected stub response in context updates")
	}
}

func TestCodergenHandlerNoPrompt(t *testing.T) {
	h := NewCodergenHandler(nil)
	node := &model.Node{ID: "gen", Attrs: map[string]string{}}
	o, err := h.Execute(context.Background(), node, runtime.NewContext(), model.NewGraph("test"), "")
	if err != nil {
		t.Fatal(err)
	}
	if o.Status != runtime.StatusFail {
		t.Fatal("expected fail for missing prompt")
	}
}

type mockBackend struct {
	response string
	err      error
}

func (m *mockBackend) Generate(_ context.Context, prompt string, opts map[string]string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return m.response, nil
}

func TestCodergenHandlerWithBackend(t *testing.T) {
	h := NewCodergenHandler(&mockBackend{response: "generated code"})
	node := &model.Node{ID: "gen", Attrs: map[string]string{"prompt": "write code"}}
	dir := t.TempDir()
	o, err := h.Execute(context.Background(), node, runtime.NewContext(), model.NewGraph("test"), dir)
	if err != nil {
		t.Fatal(err)
	}
	if o.Status != runtime.StatusSuccess {
		t.Fatalf("expected success, got %s: %s", o.Status, o.FailureReason)
	}
	if o.ContextUpdates["gen.response"] != "generated code" {
		t.Fatal("expected response in context updates")
	}
}

func TestWaitHumanHandler(t *testing.T) {
	iv := interviewer.NewQueue("yes")
	h := NewWaitHumanHandler(iv)

	g := model.NewGraph("test")
	node := &model.Node{ID: "approval", Attrs: map[string]string{"prompt": "Approve this?"}}
	g.AddNode(node)

	o, err := h.Execute(context.Background(), node, runtime.NewContext(), g, "")
	if err != nil {
		t.Fatal(err)
	}
	if o.Status != runtime.StatusSuccess {
		t.Fatal("expected success")
	}
	if o.PreferredLabel != "yes" {
		t.Fatalf("expected preferred label 'yes', got %q", o.PreferredLabel)
	}
}

func TestWaitHumanHandlerMultipleChoice(t *testing.T) {
	iv := interviewer.NewQueue("approve")
	h := NewWaitHumanHandler(iv)

	g := model.NewGraph("test")
	node := &model.Node{ID: "review", Attrs: map[string]string{"prompt": "Review result"}}
	g.AddNode(node)
	g.AddEdge(&model.Edge{From: "review", To: "next", Attrs: map[string]string{"label": "approve"}, Order: 0})
	g.AddEdge(&model.Edge{From: "review", To: "redo", Attrs: map[string]string{"label": "reject"}, Order: 1})

	o, err := h.Execute(context.Background(), node, runtime.NewContext(), g, "")
	if err != nil {
		t.Fatal(err)
	}
	if o.PreferredLabel != "approve" {
		t.Fatalf("expected 'approve', got %q", o.PreferredLabel)
	}
}

func TestFanInHandler(t *testing.T) {
	h := &FanInHandler{}
	o, err := h.Execute(context.Background(), &model.Node{ID: "join"}, runtime.NewContext(), model.NewGraph("test"), "")
	if err != nil || o.Status != runtime.StatusSuccess {
		t.Fatal("fan-in should succeed")
	}
}

func TestParallelHandlerStub(t *testing.T) {
	h := NewParallelHandler()
	g := model.NewGraph("test")
	node := &model.Node{ID: "par", Attrs: map[string]string{"join": "wait_all"}}
	g.AddNode(node)
	g.AddNode(&model.Node{ID: "a", Attrs: map[string]string{}})
	g.AddNode(&model.Node{ID: "b", Attrs: map[string]string{}})
	g.AddEdge(&model.Edge{From: "par", To: "a", Order: 0})
	g.AddEdge(&model.Edge{From: "par", To: "b", Order: 1})

	o, err := h.Execute(context.Background(), node, runtime.NewContext(), g, "")
	if err != nil || o.Status != runtime.StatusSuccess {
		t.Fatal("parallel stub should succeed")
	}
}

func TestJoinParallelResultsWaitAll(t *testing.T) {
	outcomes := map[string]runtime.Outcome{
		"a": runtime.SuccessOutcome(),
		"b": runtime.SuccessOutcome(),
	}
	o := joinParallelResults(outcomes, nil, JoinWaitAll, 0)
	if o.Status != runtime.StatusSuccess {
		t.Fatal("wait_all should succeed when all succeed")
	}

	outcomes["b"] = runtime.FailOutcome("bad", runtime.FailureTransient)
	o = joinParallelResults(outcomes, nil, JoinWaitAll, 0)
	if o.Status != runtime.StatusFail {
		t.Fatal("wait_all should fail when any fails")
	}
}

func TestJoinParallelResultsFirstSuccess(t *testing.T) {
	outcomes := map[string]runtime.Outcome{
		"a": runtime.FailOutcome("bad", runtime.FailureTransient),
		"b": runtime.SuccessOutcome(),
	}
	o := joinParallelResults(outcomes, nil, JoinFirstSuccess, 0)
	if o.Status != runtime.StatusSuccess {
		t.Fatal("first_success should succeed when at least one succeeds")
	}
}

func TestJoinParallelResultsKOfN(t *testing.T) {
	outcomes := map[string]runtime.Outcome{
		"a": runtime.SuccessOutcome(),
		"b": runtime.FailOutcome("bad", runtime.FailureTransient),
		"c": runtime.SuccessOutcome(),
	}
	o := joinParallelResults(outcomes, nil, JoinKOfN, 2)
	if o.Status != runtime.StatusSuccess {
		t.Fatal("k_of_n(2) should succeed with 2/3")
	}

	o = joinParallelResults(outcomes, nil, JoinKOfN, 3)
	if o.Status != runtime.StatusFail {
		t.Fatal("k_of_n(3) should fail with 2/3")
	}
}

func TestJoinParallelResultsQuorum(t *testing.T) {
	outcomes := map[string]runtime.Outcome{
		"a": runtime.SuccessOutcome(),
		"b": runtime.SuccessOutcome(),
		"c": runtime.FailOutcome("bad", runtime.FailureTransient),
	}
	o := joinParallelResults(outcomes, nil, JoinQuorum, 0)
	if o.Status != runtime.StatusSuccess {
		t.Fatal("quorum should succeed with 2/3")
	}
}

func TestManagerLoopHandlerStub(t *testing.T) {
	h := &ManagerLoopHandler{}
	o, err := h.Execute(context.Background(), &model.Node{ID: "loop"}, runtime.NewContext(), model.NewGraph("test"), "")
	if err != nil || o.Status != runtime.StatusSuccess {
		t.Fatal("manager_loop stub should succeed")
	}
}

func TestCodingAgentHandlerStub(t *testing.T) {
	h := NewCodingAgentHandler(nil)
	node := &model.Node{ID: "agent1", Attrs: map[string]string{"prompt": "fix the bug"}}
	o, err := h.Execute(context.Background(), node, runtime.NewContext(), model.NewGraph("test"), "")
	if err != nil {
		t.Fatal(err)
	}
	if o.Status != runtime.StatusSuccess {
		t.Fatalf("expected success, got %s: %s", o.Status, o.FailureReason)
	}
	if o.ContextUpdates["agent1.response"] != "(stub agent response)" {
		t.Error("expected stub response in context updates")
	}
}

func TestCodingAgentHandlerMissingPrompt(t *testing.T) {
	h := NewCodingAgentHandler(nil)
	node := &model.Node{ID: "agent2", Attrs: map[string]string{}}
	o, err := h.Execute(context.Background(), node, runtime.NewContext(), model.NewGraph("test"), "")
	if err != nil {
		t.Fatal(err)
	}
	if o.Status != runtime.StatusFail {
		t.Fatal("expected fail for missing prompt")
	}
}

func TestCodingAgentHandlerRegistered(t *testing.T) {
	r := DefaultRegistry()
	node := &model.Node{ID: "a", Attrs: map[string]string{"type": "coding_agent"}}
	h, err := r.Lookup(node)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := h.(*CodingAgentHandler); !ok {
		t.Fatal("expected CodingAgentHandler for type=coding_agent")
	}
}

func TestLLMCodergenBackend(t *testing.T) {
	prov := &mockLLMProvider{response: "generated"}
	client := newMockClient(prov)
	backend := NewLLMCodergenBackend(client, "test-model")
	resp, err := backend.Generate(context.Background(), "write code", nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp != "generated" {
		t.Errorf("response = %q", resp)
	}
}

func TestRegistryLookup(t *testing.T) {
	r := DefaultRegistry()

	start := &model.Node{ID: "s", Attrs: map[string]string{"shape": "Mdiamond"}}
	h, err := r.Lookup(start)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := h.(*StartHandler); !ok {
		t.Fatal("expected StartHandler for Mdiamond")
	}

	cg := &model.Node{ID: "g", Attrs: map[string]string{"type": "codergen", "shape": "box"}}
	h, err = r.Lookup(cg)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := h.(*CodergenHandler); !ok {
		t.Fatal("expected CodergenHandler for type=codergen")
	}

	unknown := &model.Node{ID: "x", Attrs: map[string]string{"shape": "star"}}
	_, err = r.Lookup(unknown)
	if err == nil {
		t.Fatal("expected error for unknown shape/type")
	}
}
