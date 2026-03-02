package handler

import (
	"context"

	"dfgo/internal/attractor/interviewer"
	"dfgo/internal/attractor/model"
	"dfgo/internal/attractor/runtime"
)

// WaitHumanHandler prompts a human via an Interviewer and routes based on their answer.
type WaitHumanHandler struct {
	Interviewer interviewer.Interviewer
}

// NewWaitHumanHandler creates a WaitHumanHandler.
func NewWaitHumanHandler(iv interviewer.Interviewer) *WaitHumanHandler {
	return &WaitHumanHandler{Interviewer: iv}
}

func (h *WaitHumanHandler) Execute(_ context.Context, node *model.Node, pctx *runtime.Context, g *model.Graph, _ string) (runtime.Outcome, error) {
	prompt := node.StringAttr("prompt", node.StringAttr("label", "Approve?"))

	// Build question from node attributes
	q := interviewer.Question{
		Prompt: prompt,
	}

	// If the node has options, use multiple choice
	opts := collectOptions(node, g)
	if len(opts) > 0 {
		q.Type = interviewer.MultipleChoice
		q.Options = opts
	} else {
		q.Type = interviewer.YesNo
	}

	ans, err := h.Interviewer.Ask(q)
	if err != nil {
		return runtime.FailOutcome("interviewer error: "+err.Error(), runtime.FailureDeterministic), nil
	}

	return runtime.Outcome{
		Status:         runtime.StatusSuccess,
		PreferredLabel: ans.Text,
		ContextUpdates: map[string]string{
			node.ID + ".answer": ans.Text,
		},
	}, nil
}

func (h *WaitHumanHandler) IsSingleExecution() bool { return true }

// collectOptions builds option labels from outgoing edge labels.
func collectOptions(node *model.Node, g *model.Graph) []string {
	edges := g.OutEdges(node.ID)
	if len(edges) == 0 {
		return nil
	}
	var opts []string
	for _, e := range edges {
		if label := e.Attrs["label"]; label != "" {
			opts = append(opts, label)
		}
	}
	if len(opts) < 2 {
		return nil // need at least 2 options for multiple choice
	}
	return opts
}
