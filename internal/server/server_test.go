package server

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"dfgo/internal/attractor"
	"dfgo/internal/server/runmgr"
)

const simplePipeline = `digraph simple {
	start [shape=Mdiamond]
	end [shape=Msquare]
	start -> end
}`

const humanGatePipeline = `digraph gate {
	start [shape=Mdiamond]
	gate [type="wait.human" prompt="Approve?" label="Approve?"]
	end [shape=Msquare]
	start -> gate -> end
}`

func testServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := New(Config{
		ManagerCfg: runmgr.ManagerConfig{
			BaseCfg: attractor.EngineConfig{
				LogsDir: t.TempDir(),
			},
		},
	})
	return httptest.NewServer(srv.Handler())
}

func TestHealthEndpoint(t *testing.T) {
	ts := testServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/health")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var body HealthResponse
	json.NewDecoder(resp.Body).Decode(&body)
	if body.Status != "ok" {
		t.Errorf("expected 'ok', got %q", body.Status)
	}
}

func TestSubmitAndStatus(t *testing.T) {
	ts := testServer(t)
	defer ts.Close()

	// Submit pipeline.
	body := `{"dot_source":"` + escapeJSON(simplePipeline) + `","auto_approve":true}`
	resp, err := http.Post(ts.URL+"/api/v1/pipelines", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 202, got %d: %s", resp.StatusCode, b)
	}

	var submitResp SubmitResponse
	json.NewDecoder(resp.Body).Decode(&submitResp)
	if submitResp.RunID == "" {
		t.Fatal("expected non-empty run_id")
	}

	// Wait for completion.
	time.Sleep(200 * time.Millisecond)

	// Check status.
	statusResp, err := http.Get(ts.URL + "/api/v1/pipelines/" + submitResp.RunID)
	if err != nil {
		t.Fatal(err)
	}
	defer statusResp.Body.Close()

	var status StatusResponse
	json.NewDecoder(statusResp.Body).Decode(&status)
	if status.Status != "completed" {
		t.Errorf("expected completed, got %q (error: %s)", status.Status, status.Error)
	}
	if status.Pipeline != "simple" {
		t.Errorf("expected pipeline name 'simple', got %q", status.Pipeline)
	}
}

func TestSubmitInvalidDOT(t *testing.T) {
	ts := testServer(t)
	defer ts.Close()

	body := `{"dot_source":"not valid dot"}`
	resp, err := http.Post(ts.URL+"/api/v1/pipelines", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d", resp.StatusCode)
	}
}

func TestSubmitMissingDOTSource(t *testing.T) {
	ts := testServer(t)
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/api/v1/pipelines", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestSSEEventStream(t *testing.T) {
	ts := testServer(t)
	defer ts.Close()

	// Submit pipeline.
	body := `{"dot_source":"` + escapeJSON(simplePipeline) + `","auto_approve":true}`
	resp, err := http.Post(ts.URL+"/api/v1/pipelines", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	var submitResp SubmitResponse
	json.NewDecoder(resp.Body).Decode(&submitResp)
	resp.Body.Close()

	// Wait for completion so all events are in history.
	time.Sleep(200 * time.Millisecond)

	// Connect SSE.
	sseResp, err := http.Get(ts.URL + "/api/v1/pipelines/" + submitResp.RunID + "/events")
	if err != nil {
		t.Fatal(err)
	}
	defer sseResp.Body.Close()

	if ct := sseResp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("expected text/event-stream, got %q", ct)
	}

	// Read events.
	var eventTypes []string
	scanner := bufio.NewScanner(sseResp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "event: ") {
			eventTypes = append(eventTypes, strings.TrimPrefix(line, "event: "))
		}
	}

	// Should have pipeline.started, stage events, pipeline.completed, checkpoint, and done.
	if len(eventTypes) < 3 {
		t.Errorf("expected at least 3 event types, got %d: %v", len(eventTypes), eventTypes)
	}

	// Last event should be "done".
	if eventTypes[len(eventTypes)-1] != "done" {
		t.Errorf("expected last event to be 'done', got %q", eventTypes[len(eventTypes)-1])
	}
}

func TestContextEndpoint(t *testing.T) {
	ts := testServer(t)
	defer ts.Close()

	body := `{"dot_source":"` + escapeJSON(simplePipeline) + `","auto_approve":true}`
	resp, err := http.Post(ts.URL+"/api/v1/pipelines", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	var submitResp SubmitResponse
	json.NewDecoder(resp.Body).Decode(&submitResp)
	resp.Body.Close()

	time.Sleep(200 * time.Millisecond)

	ctxResp, err := http.Get(ts.URL + "/api/v1/pipelines/" + submitResp.RunID + "/context")
	if err != nil {
		t.Fatal(err)
	}
	defer ctxResp.Body.Close()

	if ctxResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", ctxResp.StatusCode)
	}

	var ctxBody ContextResponse
	json.NewDecoder(ctxResp.Body).Decode(&ctxBody)
	if ctxBody.RunID != submitResp.RunID {
		t.Errorf("expected run_id %q, got %q", submitResp.RunID, ctxBody.RunID)
	}
	if ctxBody.Context == nil {
		t.Error("expected non-nil context map")
	}
}

func TestCancelEndpoint(t *testing.T) {
	ts := testServer(t)
	defer ts.Close()

	// Submit a blocking pipeline.
	body := `{"dot_source":"` + escapeJSON(humanGatePipeline) + `"}`
	resp, err := http.Post(ts.URL+"/api/v1/pipelines", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	var submitResp SubmitResponse
	json.NewDecoder(resp.Body).Decode(&submitResp)
	resp.Body.Close()

	time.Sleep(100 * time.Millisecond)

	// Cancel it.
	cancelResp, err := http.Post(ts.URL+"/api/v1/pipelines/"+submitResp.RunID+"/cancel", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cancelResp.Body.Close()

	if cancelResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", cancelResp.StatusCode)
	}

	time.Sleep(200 * time.Millisecond)

	// Check status.
	statusResp, err := http.Get(ts.URL + "/api/v1/pipelines/" + submitResp.RunID)
	if err != nil {
		t.Fatal(err)
	}
	defer statusResp.Body.Close()

	var status StatusResponse
	json.NewDecoder(statusResp.Body).Decode(&status)
	if status.Status != "canceled" && status.Status != "failed" {
		t.Errorf("expected canceled or failed, got %q", status.Status)
	}
}

func TestNotFound(t *testing.T) {
	ts := testServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/pipelines/nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestHumanGateQuestionAndAnswer(t *testing.T) {
	ts := testServer(t)
	defer ts.Close()

	// Submit pipeline with human gate (not auto-approve).
	body := `{"dot_source":"` + escapeJSON(humanGatePipeline) + `"}`
	resp, err := http.Post(ts.URL+"/api/v1/pipelines", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	var submitResp SubmitResponse
	json.NewDecoder(resp.Body).Decode(&submitResp)
	resp.Body.Close()

	// Wait for question to appear.
	var questions []QuestionResponse
	deadline := time.After(2 * time.Second)
	for {
		qResp, err := http.Get(ts.URL + "/api/v1/pipelines/" + submitResp.RunID + "/questions")
		if err != nil {
			t.Fatal(err)
		}
		json.NewDecoder(qResp.Body).Decode(&questions)
		qResp.Body.Close()
		if len(questions) > 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for pending question")
		default:
			time.Sleep(50 * time.Millisecond)
		}
	}

	if questions[0].Prompt != "Approve?" {
		t.Errorf("expected prompt 'Approve?', got %q", questions[0].Prompt)
	}

	// Answer the question.
	answerBody := `{"text":"yes","selected":-1}`
	ansResp, err := http.Post(
		ts.URL+"/api/v1/pipelines/"+submitResp.RunID+"/questions/"+questions[0].ID+"/answer",
		"application/json",
		strings.NewReader(answerBody),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer ansResp.Body.Close()

	if ansResp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(ansResp.Body)
		t.Fatalf("expected 200, got %d: %s", ansResp.StatusCode, b)
	}

	// Wait for completion.
	time.Sleep(500 * time.Millisecond)

	statusResp, err := http.Get(ts.URL + "/api/v1/pipelines/" + submitResp.RunID)
	if err != nil {
		t.Fatal(err)
	}
	defer statusResp.Body.Close()

	var status StatusResponse
	json.NewDecoder(statusResp.Body).Decode(&status)
	if status.Status != "completed" {
		t.Errorf("expected completed, got %q (error: %s)", status.Status, status.Error)
	}
}

// escapeJSON escapes a string for embedding in a JSON string literal.
func escapeJSON(s string) string {
	b, _ := json.Marshal(s)
	// Strip surrounding quotes.
	return string(b[1 : len(b)-1])
}

