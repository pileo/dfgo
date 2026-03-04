package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"dfgo/internal/attractor/interviewer"
	"dfgo/internal/server/runmgr"
)

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, HealthResponse{Status: "ok"})
}

func (s *Server) handleSubmit(w http.ResponseWriter, r *http.Request) {
	var req SubmitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON", err.Error())
		return
	}
	if req.DOTSource == "" {
		writeError(w, http.StatusBadRequest, "dot_source is required", "")
		return
	}

	id, err := s.manager.Submit(r.Context(), runmgr.SubmitOptions{
		DOTSource:      req.DOTSource,
		InitialContext: req.InitialContext,
		AutoApprove:    req.AutoApprove,
		Simulate:       req.Simulate,
	})
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "pipeline submission failed", err.Error())
		return
	}

	writeJSON(w, http.StatusAccepted, SubmitResponse{RunID: id})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	run := s.lookupRun(w, r)
	if run == nil {
		return
	}

	var currentNode string
	if eng := run.Engine(); eng != nil && eng.PCtx != nil {
		currentNode, _ = eng.PCtx.Get("current_node")
	}

	snap := run.Snapshot()
	writeJSON(w, http.StatusOK, StatusResponse{
		RunID:       snap.ID,
		Status:      string(snap.Status),
		Pipeline:    snap.Pipeline,
		CurrentNode: currentNode,
		StartedAt:   snap.StartedAt,
		CompletedAt: snap.CompletedAt,
		Error:       snap.Error,
	})
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	run := s.lookupRun(w, r)
	if run == nil {
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported", "")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	sub := run.Broadcaster().Subscribe(r.Context())

	// Support Last-Event-ID for reconnection: skip events up to that ID.
	var skipUntil int
	if lastID := r.Header.Get("Last-Event-ID"); lastID != "" {
		skipUntil, _ = strconv.Atoi(lastID)
	}

	seq := 0
	for evt := range sub.C {
		seq++
		if seq <= skipUntil {
			continue
		}
		data := evt.JSON()
		fmt.Fprintf(w, "event: %s\ndata: %s\nid: %d\n\n", evt.Type, data, seq)
		flusher.Flush()
	}

	// Terminal event.
	fmt.Fprintf(w, "event: done\ndata: {}\n\n")
	flusher.Flush()
}

func (s *Server) handleCancel(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.manager.Cancel(id); err != nil {
		writeError(w, http.StatusNotFound, "run not found", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "canceled"})
}

func (s *Server) handleListQuestions(w http.ResponseWriter, r *http.Request) {
	run := s.lookupRun(w, r)
	if run == nil {
		return
	}

	iv := run.Interviewer()
	if iv == nil {
		writeJSON(w, http.StatusOK, []QuestionResponse{})
		return
	}

	pending := iv.Pending()
	resp := make([]QuestionResponse, len(pending))
	for i, pq := range pending {
		resp[i] = questionToResponse(pq)
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleAnswer(w http.ResponseWriter, r *http.Request) {
	run := s.lookupRun(w, r)
	if run == nil {
		return
	}

	iv := run.Interviewer()
	if iv == nil {
		writeError(w, http.StatusBadRequest, "run uses auto-approve, no questions to answer", "")
		return
	}

	qid := r.PathValue("qid")
	var req AnswerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON", err.Error())
		return
	}

	if err := iv.SubmitAnswer(qid, interviewer.Answer{
		Text:     req.Text,
		Selected: req.Selected,
	}); err != nil {
		writeError(w, http.StatusNotFound, "question not found", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "answered"})
}

func (s *Server) handleContext(w http.ResponseWriter, r *http.Request) {
	run := s.lookupRun(w, r)
	if run == nil {
		return
	}

	eng := run.Engine()
	if eng == nil || eng.PCtx == nil {
		writeError(w, http.StatusServiceUnavailable, "context not yet available", "")
		return
	}

	writeJSON(w, http.StatusOK, ContextResponse{
		RunID:   run.ID,
		Context: eng.PCtx.Snapshot(),
		Logs:    eng.PCtx.Logs(),
	})
}

// lookupRun extracts the run ID from the path and looks it up.
// Writes a 404 and returns nil if not found.
func (s *Server) lookupRun(w http.ResponseWriter, r *http.Request) *runmgr.Run {
	id := r.PathValue("id")
	run := s.manager.Get(id)
	if run == nil {
		writeError(w, http.StatusNotFound, "run not found", "")
		return nil
	}
	return run
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg, details string) {
	writeJSON(w, status, ErrorResponse{Error: msg, Details: details})
}

func questionToResponse(pq interviewer.PendingQuestion) QuestionResponse {
	var typeName string
	switch pq.Question.Type {
	case interviewer.YesNo:
		typeName = "yes_no"
	case interviewer.MultipleChoice:
		typeName = "multiple_choice"
	case interviewer.Freeform:
		typeName = "freeform"
	case interviewer.Confirmation:
		typeName = "confirmation"
	default:
		typeName = "unknown"
	}
	return QuestionResponse{
		ID:      pq.ID,
		Type:    typeName,
		Prompt:  pq.Question.Prompt,
		Options: pq.Question.Options,
		Default: pq.Question.Default,
	}
}
