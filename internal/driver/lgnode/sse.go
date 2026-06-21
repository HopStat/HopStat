package lgnode

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

func readSSE(r io.Reader, onEvent func(event, data string) error) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	var eventType string
	var dataLines []string

	flush := func() error {
		if eventType == "" && len(dataLines) == 0 {
			return nil
		}
		data := strings.Join(dataLines, "\n")
		err := onEvent(eventType, data)
		eventType = ""
		dataLines = nil
		return err
	}

	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")
		if line == "" {
			if err := flush(); err != nil {
				return err
			}
			continue
		}
		if strings.HasPrefix(line, "event:") {
			eventType = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			continue
		}
		if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}

	if err := scanner.Err(); err != nil {
		return err
	}
	return flush()
}

func parseOutputLine(data string) (string, error) {
	var payload struct {
		Line string `json:"line"`
	}
	if err := json.Unmarshal([]byte(data), &payload); err != nil {
		return "", err
	}
	return payload.Line, nil
}

func parseStreamError(data string) error {
	var payload struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal([]byte(data), &payload); err != nil {
		return fmt.Errorf("agent stream failed")
	}
	if payload.Error == "" {
		return fmt.Errorf("agent stream failed")
	}
	return fmt.Errorf("%s", payload.Error)
}
