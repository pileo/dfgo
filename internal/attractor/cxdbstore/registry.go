package cxdbstore

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"time"
)

// BundleID identifies the dfgo registry bundle in CXDB.
const BundleID = "com.attractor.dfgo-v1"

// PublishBundle publishes the type registry bundle to CXDB's HTTP API.
// httpAddr is the CXDB HTTP address (e.g., "http://localhost:9010").
// Best-effort: logs warnings on failure but does not return errors.
func PublishBundle(httpAddr string) {
	bundle := RegistryBundle()
	data, err := json.Marshal(bundle)
	if err != nil {
		slog.Warn("cxdb: failed to marshal registry bundle", "error", err)
		return
	}

	url := fmt.Sprintf("%s/v1/registry/bundles/%s", httpAddr, BundleID)
	req, err := http.NewRequest("PUT", url, bytes.NewReader(data))
	if err != nil {
		slog.Warn("cxdb: failed to create registry request", "error", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		slog.Warn("cxdb: failed to publish registry bundle", "error", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusNoContent {
		slog.Info("cxdb: registry bundle published", "bundle_id", BundleID, "status", resp.StatusCode)
	} else {
		body, _ := io.ReadAll(resp.Body)
		slog.Warn("cxdb: registry bundle publish failed", "status", resp.StatusCode, "body", string(body))
	}
}

// HTTPAddrFromBinary derives the CXDB HTTP address from the binary protocol address.
// Convention: HTTP port = binary port + 1 (e.g., 9009 → 9010).
func HTTPAddrFromBinary(binaryAddr string) string {
	host, portStr, err := net.SplitHostPort(binaryAddr)
	if err != nil {
		return "http://localhost:9010"
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return "http://localhost:9010"
	}
	return fmt.Sprintf("http://%s:%d", host, port+1)
}

// RegistryBundle returns the type registry for all dfgo turn types.
// Format matches the CXDB registry bundle schema.
func RegistryBundle() map[string]any {
	return map[string]any{
		"registry_version": 1,
		"bundle_id":        BundleID,
		"types": map[string]any{
			TypePipelineStarted: typeDesc(1, map[string]any{
				"1": field("run_id", "string"),
				"2": field("pipeline", "string"),
				"3": field("start_node", "string"),
				"4": tsField("timestamp"),
			}),
			TypePipelineCompleted: typeDesc(1, map[string]any{
				"1": field("run_id", "string"),
				"2": field("pipeline", "string"),
				"3": field("status", "string"),
				"4": tsField("timestamp"),
			}),
			TypePipelineFailed: typeDesc(1, map[string]any{
				"1": field("run_id", "string"),
				"2": field("error", "string"),
				"3": tsField("timestamp"),
			}),
			TypeStageStarted: typeDesc(1, map[string]any{
				"1": field("node_id", "string"),
				"2": field("node_type", "string"),
				"3": field("shape", "string"),
				"4": tsField("timestamp"),
			}),
			TypeStageCompleted: typeDesc(1, map[string]any{
				"1": field("node_id", "string"),
				"2": field("status", "string"),
				"3": field("notes", "string"),
				"4": tsField("timestamp"),
			}),
			TypeStageFailed: typeDesc(1, map[string]any{
				"1": field("node_id", "string"),
				"2": field("status", "string"),
				"3": field("failure_reason", "string"),
				"4": field("failure_class", "string"),
				"5": tsField("timestamp"),
			}),
			TypeStageRetrying: typeDesc(1, map[string]any{
				"1": field("node_id", "string"),
				"2": field("attempt", "i32"),
				"3": field("max_retry", "i32"),
				"4": tsField("timestamp"),
			}),
			TypeCheckpointSaved: typeDesc(1, map[string]any{
				"1": field("current_node", "string"),
				"2": field("commit_sha", "string"),
				"3": tsField("timestamp"),
			}),
			TypeParallelStarted: typeDesc(1, map[string]any{
				"1": field("node_id", "string"),
				"2": field("branch_count", "i32"),
				"3": fieldArray("branch_ids", "string"),
				"4": field("join_policy", "string"),
				"5": tsField("timestamp"),
			}),
			TypeParallelBranch: typeDesc(1, map[string]any{
				"1": field("node_id", "string"),
				"2": field("branch_key", "string"),
				"3": field("event", "string"),
				"4": field("status", "string"),
				"5": tsField("timestamp"),
			}),
			TypeParallelCompleted: typeDesc(1, map[string]any{
				"1": field("node_id", "string"),
				"2": field("winner_key", "string"),
				"3": field("join_policy", "string"),
				"4": tsField("timestamp"),
			}),
			TypeInterviewStarted: typeDesc(1, map[string]any{
				"1": field("node_id", "string"),
				"2": field("event", "string"),
				"3": field("question", "string"),
				"4": field("answer", "string"),
				"5": tsField("timestamp"),
			}),
			TypeInterviewCompleted: typeDesc(1, map[string]any{
				"1": field("node_id", "string"),
				"2": field("event", "string"),
				"3": field("question", "string"),
				"4": field("answer", "string"),
				"5": tsField("timestamp"),
			}),
			TypeInterviewTimeout: typeDesc(1, map[string]any{
				"1": field("node_id", "string"),
				"2": field("event", "string"),
				"3": field("question", "string"),
				"4": field("answer", "string"),
				"5": tsField("timestamp"),
			}),
			// Agent events use canonical cxdb.ConversationItem v3 types
			// (AssistantTurn with ToolCalls/TurnMetrics, SystemMessage for
			// turn boundaries and loop detection). No custom registry needed.
		},
	}
}

// typeDesc wraps fields in the CXDB versioned type descriptor format.
func typeDesc(version int, fields map[string]any) map[string]any {
	return map[string]any{
		"versions": map[string]any{
			strconv.Itoa(version): map[string]any{
				"fields": fields,
			},
		},
	}
}

func field(name, typ string) map[string]any {
	return map[string]any{
		"name": name,
		"type": typ,
	}
}

func fieldSemantic(name, typ, semantic string) map[string]any {
	return map[string]any{
		"name":     name,
		"type":     typ,
		"semantic": semantic,
	}
}

func tsField(name string) map[string]any {
	return fieldSemantic(name, "u64", "unix_ms")
}

func fieldArray(name, elemType string) map[string]any {
	return map[string]any{
		"name":      name,
		"type":      "array",
		"elem_type": elemType,
	}
}
