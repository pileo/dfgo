package server

import "net/http"

func (s *Server) registerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/health", s.handleHealth)
	mux.HandleFunc("POST /api/v1/pipelines", s.handleSubmit)
	mux.HandleFunc("GET /api/v1/pipelines/{id}", s.handleStatus)
	mux.HandleFunc("GET /api/v1/pipelines/{id}/events", s.handleEvents)
	mux.HandleFunc("POST /api/v1/pipelines/{id}/cancel", s.handleCancel)
	mux.HandleFunc("GET /api/v1/pipelines/{id}/questions", s.handleListQuestions)
	mux.HandleFunc("POST /api/v1/pipelines/{id}/questions/{qid}/answer", s.handleAnswer)
	mux.HandleFunc("GET /api/v1/pipelines/{id}/context", s.handleContext)
}
