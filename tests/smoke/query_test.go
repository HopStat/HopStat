//go:build smoke

package smoke

import (
	"net/http"
	"strings"
	"testing"
)

// submitQuery posts a query and returns its id, asserting the handler's immediate contract.
func submitQuery(t *testing.T, c *http.Client, command, target string) string {
	t.Helper()
	resp := doJSON(t, c, http.MethodPost, "/api/v1/query", map[string]interface{}{
		"node_id": seedNodeID, "command": command, "target": target,
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("submit %s: status = %d, body = %s", command, resp.StatusCode, decodeError(t, resp))
	}
	var accepted struct {
		QueryID   string `json:"query_id"`
		Status    string `json:"status"`
		StreamURL string `json:"stream_url"`
	}
	decodeData(t, resp, &accepted)
	if accepted.QueryID == "" {
		t.Fatal("no query_id returned")
	}
	if accepted.Status != "running" {
		t.Errorf("status = %q, want running", accepted.Status)
	}
	if want := "/api/v1/query/" + accepted.QueryID + "/stream"; accepted.StreamURL != want {
		t.Errorf("stream_url = %q, want %q", accepted.StreamURL, want)
	}
	return accepted.QueryID
}

// The whole point of the harness: handler → engine → pool → lg_node driver → stub agent →
// parser → query store → SSE, with no ICMP, no root and no external network.
func TestQueryPipelineCompletesForEveryCommand(t *testing.T) {
	c := loggedInClient(t)

	for _, command := range []string{"ping", "traceroute", "bgp_route"} {
		t.Run(command, func(t *testing.T) {
			target := smokeTarget
			if command == "bgp_route" {
				target = "1.1.1.0/24"
			}

			id := submitQuery(t, c, command, target)
			events, result := readSSEUntilResult(t, c, id)

			if status, _ := result["status"].(string); status != "done" {
				t.Fatalf("status = %v, want done (error_msg %v)", result["status"], result["error_msg"])
			}
			if !contains(events, "result") {
				t.Errorf("no result event; saw %v", events)
			}
			// The frontend listens for a `progress` event. The server never emits one — pin
			// that so the two sides cannot silently disagree about the event vocabulary.
			if contains(events, "progress") {
				t.Errorf("server emitted a progress event; the frontend and server contracts have drifted: %v", events)
			}

			// The stored result must survive the stream closing, since the frontend falls
			// back to polling this endpoint when the SSE connection drops.
			stored := doJSON(t, c, http.MethodGet, "/api/v1/query/"+id, nil)
			defer stored.Body.Close()
			if stored.StatusCode != http.StatusOK {
				t.Fatalf("stored result status = %d", stored.StatusCode)
			}
			var replay map[string]interface{}
			decodeData(t, stored, &replay)
			if replay["status"] != "done" {
				t.Errorf("stored status = %v, want done", replay["status"])
			}
		})
	}
}

// Documents a real gap, by measuring which agent endpoint the server actually reaches.
//
// internal/agent/agent.go registers only /agent/v1/{ping,traceroute,bgp/route,capabilities,
// health} — there are no /stream routes. But internal/driver/lgnode/driver.go asks for
// /agent/v1/ping/stream whenever an OnLine callback is set, which the query handler always
// sets. The driver catches the 404 and silently retries the blocking endpoint.
//
// The consequence is not that output disappears — the handler still splits the finished
// raw output into `output` events — it is that nothing arrives until the command has
// completely finished. On a standalone node the same query paints the terminal line by
// line; on a remote node the user watches an empty box and then gets everything at once.
//
// Asserting on the endpoints hit is deterministic, where asserting on timing would flake.
func TestRemoteNodeFallsBackWhenTheAgentHasNoStreamEndpoints(t *testing.T) {
	c := loggedInClient(t)

	t.Run("without stream endpoints the driver retries the blocking one", func(t *testing.T) {
		stubStreaming.Store(false)
		drainAgentHits()

		id := submitQuery(t, c, "ping", smokeTarget)
		_, result := readSSEUntilResult(t, c, id)
		if status, _ := result["status"].(string); status != "done" {
			t.Fatalf("status = %v, want done — the 404 fallback should still complete", result["status"])
		}

		hits := collectAgentHits()
		if !contains(hits, "/agent/v1/ping/stream") {
			t.Errorf("driver never tried the streaming endpoint; hits = %v", hits)
		}
		if !contains(hits, "/agent/v1/ping") {
			t.Errorf("driver did not fall back to the blocking endpoint; hits = %v", hits)
		}
	})

	t.Run("with stream endpoints the driver stays on the stream", func(t *testing.T) {
		stubStreaming.Store(true)
		defer stubStreaming.Store(false)
		drainAgentHits()

		id := submitQuery(t, c, "ping", smokeTarget)
		events, result := readSSEUntilResult(t, c, id)
		if status, _ := result["status"].(string); status != "done" {
			t.Fatalf("status = %v, want done", result["status"])
		}
		if !contains(events, "output") || !contains(events, "output_done") {
			t.Errorf("expected output and output_done; saw %v", events)
		}

		hits := collectAgentHits()
		if contains(hits, "/agent/v1/ping") {
			t.Errorf("driver fell back even though the stream endpoint answered; hits = %v", hits)
		}
	})
}

// A failing agent must surface as a clean error, and must not leak the agent URL or the
// bearer token into a message the public UI renders.
func TestAgentFailureIsSanitised(t *testing.T) {
	c := loggedInClient(t)
	stubFailing.Store(true)
	defer stubFailing.Store(false)

	id := submitQuery(t, c, "ping", smokeTarget)
	_, result := readSSEUntilResult(t, c, id)

	status, _ := result["status"].(string)
	if status != "error" {
		t.Fatalf("status = %v, want error when the agent fails", result["status"])
	}
	message, _ := result["error_msg"].(string)
	if containsAny(message, agentToken, stubAgentURL) {
		t.Errorf("error message leaks agent credentials or address: %q", message)
	}
}

// drainAgentHits clears anything recorded by earlier tests so a run starts from zero.
func drainAgentHits() {
	for {
		select {
		case <-agentHits:
		default:
			return
		}
	}
}

// collectAgentHits reads what the stub agent has seen so far without blocking.
func collectAgentHits() []string {
	var hits []string
	for {
		select {
		case hit := <-agentHits:
			hits = append(hits, hit)
		default:
			return hits
		}
	}
}

func contains(haystack []string, needle string) bool {
	for _, v := range haystack {
		if v == needle {
			return true
		}
	}
	return false
}

func containsAny(s string, needles ...string) bool {
	for _, n := range needles {
		if n != "" && strings.Contains(s, n) {
			return true
		}
	}
	return false
}
