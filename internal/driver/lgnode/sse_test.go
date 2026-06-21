package lgnode

import (
	"strings"
	"testing"
)

func TestReadSSE(t *testing.T) {
	body := strings.Join([]string{
		"event: output",
		`data: {"line":"line one"}`,
		"",
		"event: result",
		`data: {"packets_sent":5,"packets_recv":5}`,
		"",
	}, "\n")

	var outputs []string
	var result string
	err := readSSE(strings.NewReader(body), func(event, data string) error {
		switch event {
		case "output":
			line, err := parseOutputLine(data)
			if err != nil {
				return err
			}
			outputs = append(outputs, line)
		case "result":
			result = data
		}
		return nil
	})
	if err != nil {
		t.Fatalf("readSSE error: %v", err)
	}
	if len(outputs) != 1 || outputs[0] != "line one" {
		t.Fatalf("outputs = %#v", outputs)
	}
	if result == "" {
		t.Fatal("expected result event")
	}
}

func TestParseStreamError(t *testing.T) {
	err := parseStreamError(`{"error":"command failed"}`)
	if err == nil || err.Error() != "command failed" {
		t.Fatalf("parseStreamError = %v", err)
	}
}
