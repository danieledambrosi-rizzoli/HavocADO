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

	// Create package
	pkg := TaskPackage(c.cfg.User, req.SessionID, commandID, req.Info, req.CommandLine)
	if err := c.sendPacket(pkg); err != nil {
		return Task{}, fmt.Errorf("send task: %w", err)
	}

	// Record task for attribution
	cursor := c.getOutputCursor(req.SessionID)
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
			return OutputPage{}
		case <-time.After(100 * time.Millisecond):
		}

		if time.Now().After(deadline) {
			return OutputPage{}
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

	var output map[string]interface{}
	if err := json.Unmarshal(data, &output); err != nil {
		return string(data)
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