package operator

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"Havoc/pkg/logger"
)

var (
	ErrNotConnected       = errors.New("not connected")
	ErrAuthFailed         = errors.New("authentication failed")
	ErrDuplicateUser      = errors.New("duplicate user")
	ErrLoginClosed        = errors.New("teamserver closed connection during login")
	ErrAlreadyConnected   = errors.New("already connected")
)

type writeRequest struct {
	data []byte
	done chan error
}

type Client struct {
	cfg        Config
	conn       *websocket.Conn
	writeCh    chan writeRequest
	readErrCh  chan error
	closeCh    chan struct{}
	reconnectCh chan struct{}

	mu           sync.RWMutex
	status       Status
	sessions     map[string]*Session
	listeners    map[string]*Listener
	outputs      map[string]*ringBuffer
	pendingTasks map[string]*Task
	seqCounter   uint64

	// For payload generation
	payloadCh    chan *Payload
	payloadMu    sync.Mutex

	// For login synchronization
	loginDone    chan struct{}
	loginSuccess bool
	loginErr     error
	lastPkg      *Package
}

func Dial(ctx context.Context, cfg Config) (*Client, error) {
	if cfg.URL == "" {
		return nil, errors.New("teamserver URL is required")
	}
	if cfg.User == "" {
		return nil, errors.New("username is required")
	}
	if cfg.Password == "" {
		return nil, errors.New("password is required")
	}

	// Set defaults
	if cfg.ReconnectInterval == 0 {
		cfg.ReconnectInterval = time.Second
	}
	if cfg.MaxReconnectInterval == 0 {
		cfg.MaxReconnectInterval = 30 * time.Second
	}

	tlsCfg, err := cfg.TLSConfig()
	if err != nil {
		return nil, fmt.Errorf("TLS config: %w", err)
	}

	u, err := url.Parse(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}

	dialer := websocket.Dialer{
		TLSClientConfig: tlsCfg,
		HandshakeTimeout: 10 * time.Second,
	}

	conn, _, err := dialer.DialContext(ctx, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("websocket dial: %w", err)
	}
	logger.Debug("MCP: WebSocket connected to " + cfg.URL)

	c := &Client{
		cfg:          cfg,
		conn:         conn,
		writeCh:      make(chan writeRequest, 32),
		readErrCh:    make(chan error, 1),
		closeCh:      make(chan struct{}),
		reconnectCh:  make(chan struct{}, 1),
		sessions:     make(map[string]*Session),
		listeners:    make(map[string]*Listener),
		outputs:      make(map[string]*ringBuffer),
		pendingTasks: make(map[string]*Task),
		loginDone:    make(chan struct{}),
	}

	// Send login package immediately
	passwordHash := HashPassword(cfg.Password)
	loginPkg := LoginPackage(cfg.User, passwordHash)
	logger.Debug("MCP: Sending login package for user " + cfg.User + " to " + cfg.URL)
	
	// Start read and write loops FIRST
	go c.readLoop()
	go c.writeLoop()
	
	if err := c.sendPacket(loginPkg); err != nil {
		conn.Close()
		return nil, fmt.Errorf("send login: %w", err)
	}
	logger.Debug("MCP: Login package sent successfully")

	// Wait for login result
	select {
	case <-c.loginDone:
		if c.loginErr != nil {
			return nil, c.loginErr
		}
	case <-ctx.Done():
		c.Close()
		return nil, ctx.Err()
	case <-time.After(10 * time.Second):
		c.Close()
		return nil, errors.New("login timeout")
	}

	// Update status
	c.mu.Lock()
	c.status.Connected = true
	c.status.User = cfg.User
	c.status.Server = cfg.URL
	c.status.Since = time.Now()
	c.mu.Unlock()

	// Start reconnect handler if not disabled
	if !cfg.NoReconnect {
		go c.reconnectLoop(ctx)
	}

	return c, nil
}

func (c *Client) sendPacket(pkg *Package) error {
	data, err := MarshalPackage(pkg)
	if err != nil {
		return err
	}

	req := writeRequest{
		data: data,
		done: make(chan error, 1),
	}

	select {
	case c.writeCh <- req:
		return <-req.done
	case <-c.closeCh:
		return ErrNotConnected
	}
}

func (c *Client) writeLoop() {
	logger.Debug("MCP: writeLoop started")
	for {
		select {
		case req := <-c.writeCh:
			logger.Debug("MCP: writeLoop got request")
			if c.conn == nil {
				req.done <- ErrNotConnected
				continue
			}

			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			err := c.conn.WriteMessage(websocket.BinaryMessage, req.data)
			logger.Debug("MCP: writeLoop wrote message, err: " + fmt.Sprintf("%v", err))
			req.done <- err

		case <-c.closeCh:
			logger.Debug("MCP: writeLoop exiting")
			return
		}
	}
}

func (c *Client) readLoop() {
	logger.Debug("MCP: readLoop started")
	for {
		logger.Debug("MCP: readLoop waiting for message...")
		_, data, err := c.conn.ReadMessage()
		if err != nil {
			logger.Debug("MCP: Read error: " + err.Error())
			c.readErrCh <- err
			return
		}

		logger.Debug("MCP: Received package, length: " + fmt.Sprintf("%d", len(data)))
		pkg, err := UnmarshalPackage(data)
		if err != nil {
			logger.Error("Failed to unmarshal package: " + err.Error())
			continue
		}

		logger.Debug("MCP: Parsed package - Event: " + fmt.Sprintf("%d", pkg.Head.Event) + ", SubEvent: " + fmt.Sprintf("%d", pkg.Body.SubEvent))
		c.lastPkg = pkg
		c.handlePackage(pkg)
	}
}

func (c *Client) handlePackage(pkg *Package) {
	// Handle login response
	select {
	case <-c.loginDone:
		// Already logged in, process normally
		logger.Debug("MCP: handlePackage post-login - Event: " + fmt.Sprintf("%d", pkg.Head.Event) + ", SubEvent: " + fmt.Sprintf("%d", pkg.Body.SubEvent))
	default:
		// Still waiting for login
		if pkg.Head.Event == TypeInitConnection {
			switch pkg.Body.SubEvent {
			case InitSuccess:
				c.loginSuccess = true
				close(c.loginDone)
				logger.Good("MCP operator authenticated as " + c.cfg.User)
				return
			case InitError:
				msg := "authentication failed"
				if m, ok := pkg.Body.Info["Message"].(string); ok {
					msg = m
				}
				c.loginErr = fmt.Errorf("%w: %s", ErrAuthFailed, msg)
				close(c.loginDone)
				return
			}
		}
		// For any other package during login, store and continue waiting
		return
	}

	// Process replayed and live packages
	switch pkg.Head.Event {
	case TypeInitConnection:
		if pkg.Body.SubEvent == InitProfile {
			c.handleProfile(pkg)
		}
	case TypeListener:
		c.handleListener(pkg)
	case TypeSession:
		c.handleSession(pkg)
	case TypeChat:
		c.handleChat(pkg)
	case TypeTeamserver:
		c.handleTeamserver(pkg)
	case TypeGate:
		c.handleGate(pkg)
	}
}

func (c *Client) handleProfile(pkg *Package) {
	if name, ok := pkg.Body.Info["Name"].(string); ok {
		c.mu.Lock()
		c.status.Profile = name
		c.mu.Unlock()
	}
}

func (c *Client) handleListener(pkg *Package) {
	info := ParsePackageInfo(pkg.Body.Info)
	name := info["Name"]
	if name == "" {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	listener := c.listeners[name]
	if listener == nil {
		listener = &Listener{Name: name}
		c.listeners[name] = listener
	}

	switch pkg.Body.SubEvent {
	case ListenerAdd, ListenerEdit:
		listener.Protocol = info["Protocol"]
		listener.Status = info["Status"]
		
		// Handle both event formats:
		// 1. Full format from events.Listener.ListenerAdd: Hosts, HostBind, PortBind, PortConn, Secure
		// 2. Simplified format from ListenerStartNotify: Host, Port
		if hostBind, ok := info["HostBind"]; ok {
			listener.HostBind = hostBind
		} else if host, ok := info["Host"]; ok {
			listener.HostBind = host
		}
		
		if portBind, ok := info["PortBind"]; ok {
			listener.PortBind = portBind
		} else if port, ok := info["Port"]; ok {
			listener.PortBind = port
		}
		
		listener.Hosts = info["Hosts"]
		listener.PortConn = info["PortConn"]
		
		if secure, ok := info["Secure"]; ok {
			listener.Secure = secure == "true"
		}
	case ListenerRem:
		delete(c.listeners, name)
	case ListenerErr:
		listener.Status = "Error: " + info["Error"]
	}
}

func (c *Client) handleSession(pkg *Package) {
	info := ParsePackageInfo(pkg.Body.Info)
	demonID := info["DemonID"]
	if demonID == "" {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	switch pkg.Body.SubEvent {
	case SessNew:
		sess := c.sessions[demonID]
		if sess == nil {
			sess = &Session{ID: demonID}
			c.sessions[demonID] = sess
		}
		sess.Hostname = info["Hostname"]
		sess.Username = info["Username"]
		sess.Domain = info["Domain"]
		sess.InternalIP = info["InternalIP"]
		sess.ExternalIP = info["ExternalIP"]
		sess.OSVersion = info["OSVersion"]
		sess.OSArch = info["OSArch"]
		sess.ProcessName = info["ProcessName"]
		sess.ProcessPath = info["ProcessPath"]
		sess.PID = parseInt(info["PID"])
		sess.PPID = parseInt(info["PPID"])
		sess.Active = true
		sess.Elevated = info["Elevated"] == "true"
		sess.SleepDelay = parseInt(info["SleepDelay"])
		sess.SleepJitter = parseInt(info["SleepJitter"])
		sess.LastCallIn = info["LastCallIn"]
		sess.FirstCallIn = info["FirstCallIn"]

		// Ensure output buffer exists
		if c.outputs[demonID] == nil {
			c.outputs[demonID] = newRingBuffer(4 * 1024 * 1024 / 256) // ~4MB / avg event size
		}

	case SessRem, SessDead:
		if sess := c.sessions[demonID]; sess != nil {
			sess.Active = false
		}

	case SessOutput:
		c.handleOutput(demonID, pkg, info)
	}
}

func (c *Client) handleOutput(demonID string, pkg *Package, info map[string]string) {
	outputB64 := info["Output"]
	if outputB64 == "" {
		return
	}

	// For now, store the raw output
	event := OutputEvent{
		Time:      pkg.Head.Time,
		CommandID: parseInt(info["CommandID"]),
		TaskID:    info["TaskID"],
		Text:      outputB64, // Store base64 for now, will decode on read
	}

	buf := c.outputs[demonID]
	if buf == nil {
		buf = newRingBuffer(4 * 1024 * 1024 / 256)
		c.outputs[demonID] = buf
	}

	seq := buf.Push(event)

	// If there's a pending task for this cursor, we could notify
	// but for now we just buffer
	_ = seq
}

func (c *Client) handleChat(pkg *Package) {
	// Chat events - could be used for logging
}

func (c *Client) handleTeamserver(pkg *Package) {
	// Teamserver log events
}

func (c *Client) handleGate(pkg *Package) {
	info := ParsePackageInfo(pkg.Body.Info)

	switch pkg.Body.SubEvent {
	case GateStageless:
		// Check if this is a payload response (has PayloadArray)
		if payloadB64, ok := info["PayloadArray"]; ok && payloadB64 != "" {
			data, err := base64.StdEncoding.DecodeString(payloadB64)
			if err != nil {
				logger.Error("Failed to decode payload: " + err.Error())
				return
			}

			payload := &Payload{
				AgentType: info["AgentType"],
				Listener:  info["Listener"],
				Arch:      info["Arch"],
				Format:    info["Format"],
				FileName:  info["FileName"],
				Data:      data,
			}

			// Send to waiting caller
			c.payloadMu.Lock()
			ch := c.payloadCh
			c.payloadMu.Unlock()

			if ch != nil {
				select {
				case ch <- payload:
				default:
					logger.Warn("Payload channel full or nil, dropping payload")
				}
			}
		}
	}
}

func (c *Client) Run(ctx context.Context) error {
	// The read loop is already running in background
	// This method waits for context cancellation or connection loss
	select {
	case <-ctx.Done():
		logger.Debug("MCP: Run - context done")
		return ctx.Err()
	case err := <-c.readErrCh:
		logger.Debug("MCP: Run - read error: " + err.Error())
		c.mu.Lock()
		c.status.Connected = false
		c.status.LastError = err.Error()
		c.mu.Unlock()

		if !c.cfg.NoReconnect {
			// Signal reconnect
			select {
			case c.reconnectCh <- struct{}{}:
			default:
			}
		}
		return err
	case <-c.closeCh:
		logger.Debug("MCP: Run - closeCh")
		return ErrNotConnected
	}
}

func (c *Client) reconnectLoop(ctx context.Context) {
	interval := c.cfg.ReconnectInterval
	for {
		select {
		case <-ctx.Done():
			return
		case <-c.reconnectCh:
			// Wait before reconnecting
			select {
			case <-ctx.Done():
				return
			case <-time.After(interval):
			}

			logger.Info("MCP: Attempting to reconnect...")

			if err := c.reconnect(ctx); err != nil {
				logger.Error("MCP: Reconnect failed: " + err.Error())
				// Exponential backoff
				interval *= 2
				if interval > c.cfg.MaxReconnectInterval {
					interval = c.cfg.MaxReconnectInterval
				}
				// Signal another reconnect attempt
				select {
				case c.reconnectCh <- struct{}{}:
				default:
				}
				continue
			}

			// Reset backoff on successful reconnect
			interval = c.cfg.ReconnectInterval
			logger.Good("MCP: Reconnected successfully")
		}
	}
}

func (c *Client) reconnect(ctx context.Context) error {
	// Close existing connection
	if c.conn != nil {
		c.conn.Close()
	}

	// Clear state
	c.mu.Lock()
	c.sessions = make(map[string]*Session)
	c.listeners = make(map[string]*Listener)
	c.outputs = make(map[string]*ringBuffer)
	c.pendingTasks = make(map[string]*Task)
	c.status.Connected = false
	c.mu.Unlock()

	// Create new loginDone channel
	c.loginDone = make(chan struct{})
	c.loginSuccess = false
	c.loginErr = nil

	// Reconnect
	tlsCfg, err := c.cfg.TLSConfig()
	if err != nil {
		return err
	}

	u, err := url.Parse(c.cfg.URL)
	if err != nil {
		return err
	}

	dialer := websocket.Dialer{
		TLSClientConfig: tlsCfg,
		HandshakeTimeout: 10 * time.Second,
	}

	conn, _, err := dialer.DialContext(ctx, u.String(), nil)
	if err != nil {
		return err
	}

	c.conn = conn

	// Send login
	passwordHash := HashPassword(c.cfg.Password)
	loginPkg := LoginPackage(c.cfg.User, passwordHash)
	if err := c.sendPacket(loginPkg); err != nil {
		return err
	}

	// Wait for login
	select {
	case <-c.loginDone:
		if c.loginErr != nil {
			return c.loginErr
		}
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(10 * time.Second):
		return errors.New("reconnect login timeout")
	}

	c.mu.Lock()
	c.status.Connected = true
	c.status.Since = time.Now()
	c.status.LastError = ""
	c.mu.Unlock()

	return nil
}

func (c *Client) Close() error {
	close(c.closeCh)
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

func (c *Client) Status() Status {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.status
}

func (c *Client) Sessions() []Session {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make([]Session, 0, len(c.sessions))
	for _, s := range c.sessions {
		if s.Active {
			result = append(result, *s)
		}
	}
	return result
}

func (c *Client) Session(id string) (Session, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	s, ok := c.sessions[id]
	if !ok || !s.Active {
		return Session{}, false
	}
	return *s, true
}

func (c *Client) Listeners() []Listener {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make([]Listener, 0, len(c.listeners))
	for _, l := range c.listeners {
		result = append(result, *l)
	}
	return result
}

// StartListener sends a Listener.Add package to start a new listener.
func (c *Client) StartListener(name, protocol string, info map[string]string) error {
	pkg := ListenerAddPackage(c.cfg.User, name, protocol, info)
	return c.sendPacket(pkg)
}

// EditListener sends a Listener.Edit package to modify an existing listener.
func (c *Client) EditListener(name string, info map[string]string) error {
	pkg := ListenerEditPackage(c.cfg.User, name, info)
	return c.sendPacket(pkg)
}

// RemoveListener sends a Listener.Remove package to remove a listener.
func (c *Client) RemoveListener(name string) error {
	pkg := ListenerRemovePackage(c.cfg.User, name)
	return c.sendPacket(pkg)
}

// GeneratePayload sends a Gate.Stageless package to generate a payload and waits for the result.
func (c *Client) GeneratePayload(ctx context.Context, agentType, listener, arch, format, config string) (*Payload, error) {
	// Create a channel to receive the payload
	payloadCh := make(chan *Payload, 1)

	c.payloadMu.Lock()
	c.payloadCh = payloadCh
	c.payloadMu.Unlock()

	// Send the generation request
	pkg := GateStagelessPackage(c.cfg.User, agentType, listener, arch, format, config)
	if err := c.sendPacket(pkg); err != nil {
		c.payloadMu.Lock()
		c.payloadCh = nil
		c.payloadMu.Unlock()
		return nil, fmt.Errorf("send packet: %w", err)
	}

	// Wait for the payload or context cancellation
	select {
	case payload := <-payloadCh:
		c.payloadMu.Lock()
		c.payloadCh = nil
		c.payloadMu.Unlock()
		return payload, nil
	case <-ctx.Done():
		c.payloadMu.Lock()
		c.payloadCh = nil
		c.payloadMu.Unlock()
		return nil, ctx.Err()
	case <-time.After(5 * time.Minute):
		c.payloadMu.Lock()
		c.payloadCh = nil
		c.payloadMu.Unlock()
		return nil, fmt.Errorf("payload generation timeout")
	}
}

func parseInt(s string) int {
	var result int
	fmt.Sscanf(s, "%d", &result)
	return result
}