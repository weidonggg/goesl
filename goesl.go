// package goesl provides a client library for connecting to and interacting with
// ESL (Event Socket Library) servers. It supports authentication, command sending,
// event subscription, and real-time event handling.
package goesl

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	// ErrConnectionClosed is returned when attempting to use a connection that has been closed.
	ErrConnectionClosed = errors.New("connection closed")
	// ErrNotConnected is returned when attempting to use a connection that has not been established.
	ErrNotConnected = errors.New("not connected")
	// ErrAuthenticationFailed is returned when authentication with the server fails.
	ErrAuthenticationFailed = errors.New("authentication failed")
	// ErrInvalidResponse is returned when the server sends an invalid or unexpected response.
	ErrInvalidResponse = errors.New("invalid response")
)

// Connection represents a connection to an ESL (Event Socket Library) server.
// It manages the underlying network connection, authentication, event handling,
// and command communication.
type Connection struct {
	conn          net.Conn
	reader        *bufio.Reader
	writer        *bufio.Writer
	address       string
	password      string
	connected     bool
	authenticated bool
	mu            sync.RWMutex
	eventChan     chan *Event
	errorChan     chan error
	stopChan      chan struct{}
	eventHandler  EventHandler
	onDisconnect  func()
	logger        Logger
}

// Event represents an event received from the ESL server.
// It contains parsed headers, the raw body content, and the complete raw message.
type Event struct {
	Header map[string]string
	Body   string
	Raw    string
}

// CommandResponse represents the response to a command sent to the ESL server.
// It includes success status, reply message, headers, body content, and raw message.
type CommandResponse struct {
	OK     bool
	Reply  string
	Header map[string]string
	Body   string
	Raw    string
}

// EventHandler is a function type for handling events received from the ESL server.
type EventHandler func(*Event)

// ErrorHandler is a function type for handling errors that occur during connection operations.
type ErrorHandler func(error)

// Logger is an interface for logging debug, info, warning, and error messages.
type Logger interface {
	Debug(format string, args ...interface{})
	Info(format string, args ...interface{})
	Warn(format string, args ...interface{})
	Error(format string, args ...interface{})
}

// DefaultLogger is a no-op implementation of the Logger interface.
// It provides empty implementations for all logging methods.
type DefaultLogger struct{}

// Debug logs a debug message with the given format and arguments.
func (l *DefaultLogger) Debug(format string, args ...interface{}) {}

// Info logs an info message with the given format and arguments.
func (l *DefaultLogger) Info(format string, args ...interface{}) {}

// Warn logs a warning message with the given format and arguments.
func (l *DefaultLogger) Warn(format string, args ...interface{}) {}

// Error logs an error message with the given format and arguments.
func (l *DefaultLogger) Error(format string, args ...interface{}) {}

// NewConnection creates a new Connection instance with the specified server address and password.
// It initializes internal channels for event and error handling with default buffer sizes.
func NewConnection(address, password string) *Connection {
	return &Connection{
		address:   address,
		password:  password,
		eventChan: make(chan *Event, 500),
		errorChan: make(chan error, 50),
		stopChan:  make(chan struct{}),
		logger:    &DefaultLogger{},
	}
}

// SetLogger sets the logger for the connection. All log messages from the connection
// will be sent to the provided logger. If no logger is set, DefaultLogger is used.
func (c *Connection) SetLogger(logger Logger) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.logger = logger
}

// SetEventHandler sets the event handler for incoming events. When an event is received
// from the ESL server, the handler will be called with the event. This is in addition to
// sending events to the event channel.
func (c *Connection) SetEventHandler(handler EventHandler) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.eventHandler = handler
}

// SetOnDisconnect sets a callback function that will be called when the connection
// is disconnected unexpectedly. This can be used to implement reconnection logic
// or cleanup operations.
func (c *Connection) SetOnDisconnect(handler func()) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onDisconnect = handler
}

// Connect establishes a connection to the ESL server using the provided context.
// It performs TCP connection, reads the welcome message, and authenticates using
// the configured password. If successful, it starts the read loop for handling
// incoming events.
//
// Parameters:
//   - ctx: Context for controlling connection timeout and cancellation
//
// Returns:
//   - error: nil on success, or an error if connection or authentication fails
func (c *Connection) Connect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.connected {
		return nil
	}

	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, "tcp", c.address)
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}

	c.conn = conn
	c.reader = bufio.NewReaderSize(conn, 8192)
	c.writer = bufio.NewWriterSize(conn, 8192)
	c.connected = true

	if err := c.readWelcomeMessage(ctx); err != nil {
		c.close()
		return fmt.Errorf("failed to read welcome message: %w", err)
	}

	if err := c.authenticate(ctx); err != nil {
		c.close()
		return fmt.Errorf("authentication failed: %w", err)
	}

	c.authenticated = true

	go c.readLoop()

	return nil
}

func (c *Connection) readWelcomeMessage(ctx context.Context) error {
	readDeadline := time.Now().Add(10 * time.Second)
	if deadline, ok := ctx.Deadline(); ok {
		readDeadline = deadline
	}

	if err := c.conn.SetReadDeadline(readDeadline); err != nil {
		return fmt.Errorf("failed to set read deadline: %w", err)
	}

	for {
		line, err := c.reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				return fmt.Errorf("connection closed by server")
			}
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				return fmt.Errorf("read timeout")
			}
			return err
		}
		line = trimNewline(line)

		if len(line) == 0 {
			break
		}
	}

	if err := c.conn.SetReadDeadline(time.Time{}); err != nil {
		return fmt.Errorf("failed to clear read deadline: %w", err)
	}

	return nil
}

func (c *Connection) authenticate(ctx context.Context) error {
	readDeadline := time.Now().Add(10 * time.Second)
	if deadline, ok := ctx.Deadline(); ok {
		readDeadline = deadline
	}

	if err := c.conn.SetReadDeadline(readDeadline); err != nil {
		return fmt.Errorf("failed to set read deadline: %w", err)
	}

	authCommand := fmt.Sprintf("auth %s\n\n", c.password)

	if _, err := c.writer.WriteString(authCommand); err != nil {
		return err
	}
	if err := c.writer.Flush(); err != nil {
		return err
	}

	response, err := c.readCommandResponse()
	if err != nil {
		if err == io.EOF {
			return fmt.Errorf("connection closed during authentication")
		}
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			return fmt.Errorf("authentication timeout")
		}
		return err
	}

	if err := c.conn.SetReadDeadline(time.Time{}); err != nil {
		return fmt.Errorf("failed to clear read deadline: %w", err)
	}

	if !response.OK {
		return ErrAuthenticationFailed
	}

	return nil
}

// SendCommand sends a command to the ESL server and returns the response.
// It formats the command with proper line endings, sends it to the server,
// and reads the response.
//
// Parameters:
//   - command: The command string to send to the ESL server
//
// Returns:
//   - *CommandResponse: The response from the server containing status, reply, headers, and body
//   - error: nil on success, ErrNotConnected if not connected, or an error if sending fails
func (c *Connection) SendCommand(command string) (*CommandResponse, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.connected {
		return nil, ErrNotConnected
	}

	fullCommand := fmt.Sprintf("%s\n\n", command)
	if _, err := c.writer.WriteString(fullCommand); err != nil {
		return nil, err
	}
	if err := c.writer.Flush(); err != nil {
		return nil, err
	}

	response, err := c.readCommandResponse()
	if err != nil {
		return nil, err
	}

	return response, nil
}

// SendAPI sends an API command to the ESL server and returns the response.
// This is a convenience method that prepends "api " to the command string.
//
// Parameters:
//   - command: The API command string to send to the ESL server (without "api" prefix)
//
// Returns:
//   - *CommandResponse: The response from the server containing status, reply, headers, and body
//   - error: nil on success, or an error if the command fails
func (c *Connection) SendAPI(command string) (*CommandResponse, error) {
	apiCommand := fmt.Sprintf("api %s", command)
	return c.SendCommand(apiCommand)
}

// SubscribeEvents subscribes to specific events from the ESL server.
// If no events are specified, it subscribes to all events.
//
// Parameters:
//   - format: The format for event delivery (e.g., "plain", "json", "xml")
//   - events: Variable list of event names to subscribe to. If empty, subscribes to "ALL"
//
// Returns:
//   - *CommandResponse: The response from the server containing subscription status
//   - error: nil on success, or an error if subscription fails
func (c *Connection) SubscribeEvents(format string, events ...string) (*CommandResponse, error) {
	if len(events) == 0 {
		events = []string{"ALL"}
	}

	var eventList string
	for i, event := range events {
		if i > 0 {
			eventList += " "
		}
		eventList += event
	}

	subscribeCommand := fmt.Sprintf("event %s %s", format, eventList)
	return c.SendCommand(subscribeCommand)
}

// SubscribeEventsPlain subscribes to events in plain text format.
// This is a convenience method that calls SubscribeEvents with format="plain".
//
// Parameters:
//   - events: Variable list of event names to subscribe to. If empty, subscribes to "ALL"
//
// Returns:
//   - *CommandResponse: The response from the server containing subscription status
//   - error: nil on success, or an error if subscription fails
func (c *Connection) SubscribeEventsPlain(events ...string) (*CommandResponse, error) {
	return c.SubscribeEvents("plain", events...)
}

// SubscribeEventsJSON subscribes to events in JSON format.
// This is a convenience method that calls SubscribeEvents with format="json".
//
// Parameters:
//   - events: Variable list of event names to subscribe to. If empty, subscribes to "ALL"
//
// Returns:
//   - *CommandResponse: The response from the server containing subscription status
//   - error: nil on success, or an error if subscription fails
func (c *Connection) SubscribeEventsJSON(events ...string) (*CommandResponse, error) {
	return c.SubscribeEvents("json", events...)
}

// ReadEvent reads and returns the next event from the event channel.
// It blocks until an event is available, an error occurs, or the connection is closed.
//
// Returns:
//   - *Event: The event received from the ESL server
//   - error: ErrConnectionClosed if the connection is closed, or an error from the error channel
func (c *Connection) ReadEvent() (*Event, error) {
	select {
	case <-c.stopChan:
		return nil, ErrConnectionClosed
	case event := <-c.eventChan:
		return event, nil
	case err := <-c.errorChan:
		return nil, err
	}
}

// ReadEventWithContext reads and returns the next event from the event channel with context support.
// It blocks until an event is available, an error occurs, the connection is closed, or the context is canceled.
//
// Parameters:
//   - ctx: Context for controlling the read timeout and cancellation
//
// Returns:
//   - *Event: The event received from the ESL server
//   - error: ErrConnectionClosed if the connection is closed, ctx.Err() if context is canceled, or an error from the error channel
func (c *Connection) ReadEventWithContext(ctx context.Context) (*Event, error) {
	select {
	case <-c.stopChan:
		return nil, ErrConnectionClosed
	case <-ctx.Done():
		return nil, ctx.Err()
	case event := <-c.eventChan:
		return event, nil
	case err := <-c.errorChan:
		return nil, err
	}
}

// EventChannel returns the read-only channel for receiving events.
// Events are sent to this channel as they are received from the ESL server.
//
// Returns:
//   - <-chan *Event: A read-only channel that receives events
func (c *Connection) EventChannel() <-chan *Event {
	return c.eventChan
}

// ErrorChannel returns the read-only channel for receiving errors.
// Errors that occur during connection operations are sent to this channel.
//
// Returns:
//   - <-chan error: A read-only channel that receives errors
func (c *Connection) ErrorChannel() <-chan error {
	return c.errorChan
}

// IsConnected returns the current connection status.
//
// Returns:
//   - bool: true if the connection is established, false otherwise
func (c *Connection) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected
}

// IsAuthenticated returns the current authentication status.
//
// Returns:
//   - bool: true if the connection is authenticated, false otherwise
func (c *Connection) IsAuthenticated() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.authenticated
}

// Close closes the connection to the ESL server.
// It stops the read loop, closes the network connection, and cleans up resources.
// This method is safe to call multiple times.
//
// Returns:
//   - error: nil on success, or an error if closing the connection fails
func (c *Connection) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.close()
}

func (c *Connection) close() error {
	select {
	case <-c.stopChan:
	default:
		close(c.stopChan)
	}

	var err error
	if c.conn != nil {
		err = c.conn.Close()
		c.conn = nil
	}

	c.connected = false
	c.authenticated = false
	return err
}

func (c *Connection) readLoop() {
	defer func() {
		if r := recover(); r != nil {
			c.logger.Error("Read loop panic: %v", r)
		}
	}()

	for {
		select {
		case <-c.stopChan:
			return
		default:
			event, err := c.readEvent()
			if err != nil {
				c.handleDisconnect()
				return
			}

			if event != nil {
				select {
				case c.eventChan <- event:
				default:
				}

				if c.eventHandler != nil {
					go c.eventHandler(event)
				}
			}
		}
	}
}

func (c *Connection) readEvent() (*Event, error) {
	line, err := c.reader.ReadString('\n')
	if err != nil {
		return nil, err
	}

	line = trimNewline(line)

	if len(line) == 0 {
		return nil, nil
	}

	event := &Event{
		Header: make(map[string]string),
		Raw:    line,
	}

	var contentLength int

	if strings.HasPrefix(line, "Content-Length:") {
		if cl, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "Content-Length:"))); err == nil {
			contentLength = cl
		}
		if err := parseHeaders(c.reader, event.Header); err != nil {
			return event, nil
		}
	} else {
		event.Header["Event-Name"] = line
		if err := parseHeaders(c.reader, event.Header); err != nil {
			return event, nil
		}
		if cl, ok := event.Header["Content-Length"]; ok {
			if length, err := strconv.Atoi(cl); err == nil {
				contentLength = length
			}
		}
	}

	if contentLength > 0 {
		body, err := readBody(c.reader, contentLength)
		if err == nil && len(body) > 0 {
			event.Body = body
			event.Raw += "\n\n" + body

			bodyLines := strings.Split(body, "\n")
			for _, bodyLine := range bodyLines {
				bodyLine = strings.TrimSpace(bodyLine)
				if strings.Contains(bodyLine, ":") {
					parts := strings.SplitN(bodyLine, ":", 2)
					if len(parts) == 2 {
						key := strings.TrimSpace(parts[0])
						value := strings.TrimSpace(parts[1])
						if key == "Event-Name" && event.Header["Event-Name"] == "" {
							event.Header["Event-Name"] = value
						} else {
							event.Header[key] = value
						}
					}
				}
			}
		}
	}

	return event, nil
}

func (c *Connection) readCommandResponse() (*CommandResponse, error) {
	line, err := c.reader.ReadString('\n')
	if err != nil {
		return nil, err
	}

	line = trimNewline(line)

	response := &CommandResponse{
		Header: make(map[string]string),
		Raw:    line,
	}

	if strings.Contains(line, ":") {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 {
			response.Header[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	} else {
		response.OK, response.Reply = parseReplyText(line)
	}

	if err := parseHeaders(c.reader, response.Header); err != nil {
		return response, nil
	}

	if replyText, ok := response.Header["Reply-Text"]; ok {
		response.OK, response.Reply = parseReplyText(replyText)
	}

	contentLength := 0
	if cl, ok := response.Header["Content-Length"]; ok {
		if length, err := strconv.Atoi(cl); err == nil {
			contentLength = length
		}
	}

	if contentLength > 0 {
		body, err := readBody(c.reader, contentLength)
		if err == nil && len(body) > 0 {
			response.Body = body
			response.Raw += "\n\n" + body
		}
	}

	return response, nil
}

func (c *Connection) handleDisconnect() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.connected {
		c.connected = false
		c.authenticated = false

		if c.onDisconnect != nil {
			go c.onDisconnect()
		}
	}
}

func trimNewline(s string) string {
	s = strings.TrimSuffix(s, "\r\n")
	s = strings.TrimSuffix(s, "\n")
	s = strings.TrimSuffix(s, "\r")
	return s
}

func parseHeaders(reader *bufio.Reader, headerMap map[string]string) error {
	for {
		headerLine, err := reader.ReadString('\n')
		if err != nil {
			return err
		}
		headerLine = trimNewline(headerLine)

		if len(headerLine) == 0 {
			break
		}

		if strings.Contains(headerLine, ":") {
			parts := strings.SplitN(headerLine, ":", 2)
			if len(parts) == 2 {
				key := strings.TrimSpace(parts[0])
				value := strings.TrimSpace(parts[1])
				headerMap[key] = value
			}
		}
	}
	return nil
}

func parseReplyText(reply string) (ok bool, text string) {
	if strings.HasPrefix(reply, "+OK") {
		return true, strings.TrimSpace(reply[3:])
	}
	if strings.HasPrefix(reply, "-ERR") {
		return false, strings.TrimSpace(reply[4:])
	}
	return false, reply
}

func buildCommand(base string, parts ...string) string {
	if len(parts) == 0 {
		return base
	}
	return fmt.Sprintf("%s %s", base, strings.Join(parts, " "))
}

func readBody(reader *bufio.Reader, contentLength int) (string, error) {
	if contentLength <= 0 {
		return "", nil
	}
	body := make([]byte, contentLength)
	n, err := io.ReadFull(reader, body)
	if err != nil || n == 0 {
		return "", err
	}
	return string(body[:n]), nil
}
