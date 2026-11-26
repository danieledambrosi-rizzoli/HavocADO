package operator

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"
)

// SendTask sends a task to an agent and returns the task info.
func (c *Client) SendTask(req TaskRequest) (Task, error) {
	// Prepare task with generated TaskID
	req = PrepareTaskRequest(req)

	// Extract command ID from info
	commandID := req.CommandID
	if commandID == 0 {
		if s, ok := req.Info["CommandID"]; ok {
			fmt.Sscanf(s, "%d", &commandID)
		}
	}

	// Capture the output cursor before sending so a fast agent response cannot
	// arrive between the packet write and cursor capture.
	cursor := c.getOutputCursor(req.SessionID)

	// Create package
	pkg := TaskPackage(c.cfg.User, req.SessionID, commandID, req.Info, req.CommandLine)
	if err := c.sendPacket(pkg); err != nil {
		return Task{}, fmt.Errorf("send task: %w", err)
	}

	// Record task for attribution
	task := Task{
		TaskID:       req.Info["TaskID"],
		SessionID:    req.SessionID,
		CommandID:    commandID,
		Info:         req.Info,
		CommandLine:  req.CommandLine,
		CursorAtSend: cursor,
		SentAt:       time.Now(),
	}

	c.mu.Lock()
	c.pendingTasks[task.TaskID] = &task
	c.mu.Unlock()

	return task, nil
}

func (c *Client) getOutputCursor(sessionID string) uint64 {
	c.mu.RLock()
	buf := c.outputs[sessionID]
	c.mu.RUnlock()

	if buf == nil {
		return 0
	}
	// Get latest sequence
	if event, ok := buf.Latest(); ok {
		return event.Seq
	}
	return 0
}

// ReadOutput reads buffered output for a session since the given cursor.
func (c *Client) ReadOutput(sessionID string, cursor uint64, maxBytes int) OutputPage {
	c.mu.RLock()
	buf := c.outputs[sessionID]
	c.mu.RUnlock()

	if buf == nil {
		return OutputPage{}
	}

	page := buf.GetSince(cursor, maxBytes)

	// Decode base64 output text
	for i := range page.Events {
		page.Events[i].Text = decodeOutput(page.Events[i].Text)
	}

	return page
}

// WaitOutput blocks until new output is available or timeout.
func (c *Client) WaitOutput(ctx context.Context, sessionID string, cursor uint64, timeout time.Duration) OutputPage {
	deadline := time.Now().Add(timeout)
	for {
		page := c.ReadOutput(sessionID, cursor, 1024*1024)
		if len(page.Events) > 0 {
			return page
		}

		// Wait a bit before checking again
		select {
		case <-ctx.Done():
			return OutputPage{NextCursor: cursor}
		case <-time.After(100 * time.Millisecond):
		}

		if time.Now().After(deadline) {
			return OutputPage{NextCursor: cursor}
		}
	}
}

// WaitOutputUntil blocks until output matching the predicate is available or
// the timeout expires. All events observed before the match are returned.
func (c *Client) WaitOutputUntil(ctx context.Context, sessionID string, cursor uint64, timeout time.Duration, match func(OutputEvent) bool) OutputPage {
	deadline := time.Now().Add(timeout)
	events := make([]OutputEvent, 0)
	nextCursor := cursor

	for {
		page := c.ReadOutput(sessionID, nextCursor, 1024*1024)
		for _, event := range page.Events {
			events = append(events, event)
			nextCursor = event.Seq
			if match != nil && match(event) {
				return OutputPage{
					Events:     events,
					NextCursor: nextCursor,
					Truncated:  page.Truncated,
				}
			}
		}

		select {
		case <-ctx.Done():
			return OutputPage{Events: events, NextCursor: nextCursor}
		case <-time.After(100 * time.Millisecond):
		}

		if time.Now().After(deadline) {
			return OutputPage{Events: events, NextCursor: nextCursor}
		}
	}
}

// GetTask retrieves a task by ID with its output.
func (c *Client) GetTask(taskID string) (Task, OutputPage, bool) {
	c.mu.RLock()
	task, ok := c.pendingTasks[taskID]
	c.mu.RUnlock()

	if !ok {
		return Task{}, OutputPage{}, false
	}

	// Get output since the task was sent
	page := c.ReadOutput(task.SessionID, task.CursorAtSend, 1024*1024)

	return *task, page, true
}

// decodeOutput decodes the base64-encoded JSON output from the agent.
func decodeOutput(b64 string) string {
	// The output is base64 of a JSON object
	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return b64 // Return as-is if not base64
	}

	var output map[string]any
	if err := json.Unmarshal(data, &output); err != nil {
		return string(data)
	}

	// Preserve the screenshot envelope so the MCP layer can expose its
	// base64 payload and metadata without decoding it as ordinary text.
	if miscType, ok := output["MiscType"].(string); ok && miscType == "screenshot" {
		encoded, err := json.Marshal(output)
		if err == nil {
			return string(encoded)
		}
	}

	// Prefer Output field, then Message
	if out, ok := output["Output"].(string); ok && out != "" {
		return out
	}
	if msg, ok := output["Message"].(string); ok && msg != "" {
		return msg
	}
	if typ, ok := output["Type"].(float64); ok {
		return fmt.Sprintf("[Type: %d] %s", int(typ), output["Message"])
	}

	return string(data)
}
