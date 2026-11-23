package operator

import (
	"sync"
	"time"
)

// Status represents the connection status of the operator client.
type Status struct {
	Connected  bool
	User       string
	Server     string
	Profile    string
	LastError  string
	Since      time.Time
}

// Session represents a Havoc agent session.
type Session struct {
	ID            string
	Hostname      string
	Username      string
	Domain        string
	InternalIP    string
	ExternalIP    string
	OSVersion     string
	OSArch        string
	ProcessName   string
	ProcessPath   string
	PID           int
	PPID          int
	Active        bool
	Elevated      bool
	SleepDelay    int
	SleepJitter   int
	LastCallIn    string
	FirstCallIn   string
}

// Listener represents a Havoc listener.
type Listener struct {
	Name       string
	Protocol   string
	Status     string
	Hosts      string
	HostBind   string
	PortBind   string
	PortConn   string
	Secure     bool
}

// OutputEvent represents a single output event from a session.
type OutputEvent struct {
	Seq       uint64
	Time      string
	CommandID int
	Text      string
	TaskID    string
}

// OutputPage represents a page of output events.
type OutputPage struct {
	Events     []OutputEvent
	NextCursor uint64
	Truncated  bool
}

// Task represents a task sent to an agent.
type Task struct {
	TaskID       string
	SessionID    string
	CommandID    int
	Info         map[string]string
	CommandLine  string
	CursorAtSend uint64
	SentAt       time.Time
}

// ringBuffer is a bounded ring buffer for output events.
type ringBuffer struct {
	buf     []OutputEvent
	size    int
	head    int
	tail    int
	count   int
	seq     uint64
	mu      sync.Mutex
}

func newRingBuffer(capacity int) *ringBuffer {
	return &ringBuffer{
		buf:  make([]OutputEvent, capacity),
		size: capacity,
	}
}

func (r *ringBuffer) Push(event OutputEvent) (pushedSeq uint64) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.seq++
	event.Seq = r.seq

	r.buf[r.tail] = event
	r.tail = (r.tail + 1) % r.size
	if r.count < r.size {
		r.count++
	} else {
		r.head = (r.head + 1) % r.size
	}
	return r.seq
}

func (r *ringBuffer) GetSince(cursor uint64, maxBytes int) OutputPage {
	r.mu.Lock()
	defer r.mu.Unlock()

	var events []OutputEvent
	var totalBytes int
	nextCursor := cursor

	// Iterate from oldest to newest
	for i := 0; i < r.count; i++ {
		idx := (r.head + i) % r.size
		event := r.buf[idx]
		if event.Seq <= cursor {
			continue
		}

		eventBytes := len(event.Text) + 64 // approximate
		if totalBytes+eventBytes > maxBytes && len(events) > 0 {
			break
		}

		events = append(events, event)
		totalBytes += eventBytes
		nextCursor = event.Seq
	}

	truncated := len(events) < r.count

	return OutputPage{
		Events:     events,
		NextCursor: nextCursor,
		Truncated:  truncated,
	}
}

func (r *ringBuffer) Latest() (OutputEvent, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.count == 0 {
		return OutputEvent{}, false
	}

	idx := (r.tail - 1 + r.size) % r.size
	return r.buf[idx], true
}

func (r *ringBuffer) Len() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.count
}

func (r *ringBuffer) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.head = 0
	r.tail = 0
	r.count = 0
	r.seq = 0
}