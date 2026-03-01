package goesl

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

type testLogger struct {
	debugCalled bool
	infoCalled  bool
	warnCalled  bool
	errorCalled bool
}

func (l *testLogger) Debug(format string, args ...interface{}) {
	l.debugCalled = true
}

func (l *testLogger) Info(format string, args ...interface{}) {
	l.infoCalled = true
}

func (l *testLogger) Warn(format string, args ...interface{}) {
	l.warnCalled = true
}

func (l *testLogger) Error(format string, args ...interface{}) {
	l.errorCalled = true
}

// TestNewConnection tests creating a new connection with address and password
func TestNewConnection(t *testing.T) {
	conn := NewConnection("127.0.0.1:8021", "testpass")

	if conn == nil {
		t.Fatal("NewConnection returned nil")
	}

	if conn.address != "127.0.0.1:8021" {
		t.Errorf("Expected address 127.0.0.1:8021, got %s", conn.address)
	}

	if conn.password != "testpass" {
		t.Errorf("Expected password testpass, got %s", conn.password)
	}

	if conn.connected {
		t.Error("Expected connected to be false for new connection")
	}

	if conn.authenticated {
		t.Error("Expected authenticated to be false for new connection")
	}
}

// TestSetLogger tests setting a custom logger for the connection
func TestSetLogger(t *testing.T) {
	conn := NewConnection("127.0.0.1:8021", "testpass")
	logger := &testLogger{}

	conn.SetLogger(logger)

	if conn.logger != logger {
		t.Error("Logger was not set correctly")
	}
}

// TestSetEventHandler tests setting a custom event handler for the connection
func TestSetEventHandler(t *testing.T) {
	conn := NewConnection("127.0.0.1:8021", "testpass")
	handlerCalled := false

	conn.SetEventHandler(func(event *Event) {
		handlerCalled = true
	})

	if conn.eventHandler == nil {
		t.Error("Event handler was not set")
	}

	if handlerCalled {
		t.Error("Event handler should not be called immediately")
	}
}

// TestSetOnDisconnect tests setting a callback function for disconnect events
func TestSetOnDisconnect(t *testing.T) {
	conn := NewConnection("127.0.0.1:8021", "testpass")
	disconnectCalled := false

	conn.SetOnDisconnect(func() {
		disconnectCalled = true
	})

	if conn.onDisconnect == nil {
		t.Error("On disconnect handler was not set")
	}

	if disconnectCalled {
		t.Error("On disconnect handler should not be called immediately")
	}
}

// TestIsConnected tests checking the connection status
func TestIsConnected(t *testing.T) {
	conn := NewConnection("127.0.0.1:8021", "testpass")

	if conn.IsConnected() {
		t.Error("Expected IsConnected to return false for new connection")
	}

	conn.mu.Lock()
	conn.connected = true
	conn.mu.Unlock()

	if !conn.IsConnected() {
		t.Error("Expected IsConnected to return true after setting connected")
	}
}

// TestIsAuthenticated tests checking the authentication status
func TestIsAuthenticated(t *testing.T) {
	conn := NewConnection("127.0.0.1:8021", "testpass")

	if conn.IsAuthenticated() {
		t.Error("Expected IsAuthenticated to return false for new connection")
	}

	conn.mu.Lock()
	conn.authenticated = true
	conn.mu.Unlock()

	if !conn.IsAuthenticated() {
		t.Error("Expected IsAuthenticated to return true after setting authenticated")
	}
}

// TestEventChannel tests retrieving the event channel
func TestEventChannel(t *testing.T) {
	conn := NewConnection("127.0.0.1:8021", "testpass")

	channel := conn.EventChannel()

	if channel == nil {
		t.Fatal("EventChannel returned nil")
	}

	if channel != conn.eventChan {
		t.Error("EventChannel did not return the eventChan")
	}
}

// TestErrorChannel tests retrieving the error channel
func TestErrorChannel(t *testing.T) {
	conn := NewConnection("127.0.0.1:8021", "testpass")

	channel := conn.ErrorChannel()

	if channel == nil {
		t.Fatal("ErrorChannel returned nil")
	}

	if channel != conn.errorChan {
		t.Error("ErrorChannel did not return the errorChan")
	}
}

// TestClose tests closing the connection
func TestClose(t *testing.T) {
	conn := NewConnection("127.0.0.1:8021", "testpass")

	conn.mu.Lock()
	conn.connected = true
	conn.mu.Unlock()

	err := conn.Close()

	if err != nil {
		t.Errorf("Close returned unexpected error: %v", err)
	}

	if conn.IsConnected() {
		t.Error("Expected connection to be closed")
	}
}

// TestCloseMultipleTimes tests that closing the connection multiple times is safe
func TestCloseMultipleTimes(t *testing.T) {
	conn := NewConnection("127.0.0.1:8021", "testpass")

	err1 := conn.Close()
	err2 := conn.Close()

	if err1 != nil {
		t.Errorf("First Close returned unexpected error: %v", err1)
	}

	if err2 != nil {
		t.Errorf("Second Close returned unexpected error: %v", err2)
	}
}

// TestSendCommandNotConnected tests that sending commands fails when not connected
func TestSendCommandNotConnected(t *testing.T) {
	conn := NewConnection("127.0.0.1:8021", "testpass")

	_, err := conn.SendCommand("status")

	if err != ErrNotConnected {
		t.Errorf("Expected ErrNotConnected, got %v", err)
	}
}

// TestSendAPI tests sending API commands to the server
func TestSendAPI(t *testing.T) {
	conn := NewConnection("127.0.0.1:8021", "testpass")

	var buf bytes.Buffer
	conn.mu.Lock()
	conn.connected = true
	conn.reader = bufio.NewReader(strings.NewReader("+OK\r\n\r\n"))
	conn.writer = bufio.NewWriter(&buf)
	conn.mu.Unlock()

	response, err := conn.SendAPI("status")

	if err != nil {
		t.Errorf("SendAPI returned unexpected error: %v", err)
	}

	if response == nil {
		t.Fatal("SendAPI returned nil response")
	}

	if !response.OK {
		t.Error("Expected response.OK to be true")
	}

	written := buf.String()
	if !strings.Contains(written, "api status") {
		t.Errorf("Expected command to contain 'api status', got: %s", written)
	}
}

// TestSubscribeEvents tests subscribing to server events
func TestSubscribeEvents(t *testing.T) {
	conn := NewConnection("127.0.0.1:8021", "testpass")

	var buf bytes.Buffer
	conn.mu.Lock()
	conn.connected = true
	conn.reader = bufio.NewReader(strings.NewReader("+OK\r\n\r\n"))
	conn.writer = bufio.NewWriter(&buf)
	conn.mu.Unlock()

	response, err := conn.SubscribeEvents("plain", "ALL")

	if err != nil {
		t.Errorf("SubscribeEvents returned unexpected error: %v", err)
	}

	if response == nil {
		t.Fatal("SubscribeEvents returned nil response")
	}
}

// TestSubscribeEventsPlain tests subscribing to plain text events
func TestSubscribeEventsPlain(t *testing.T) {
	conn := NewConnection("127.0.0.1:8021", "testpass")

	var buf bytes.Buffer
	conn.mu.Lock()
	conn.connected = true
	conn.reader = bufio.NewReader(strings.NewReader("+OK\r\n\r\n"))
	conn.writer = bufio.NewWriter(&buf)
	conn.mu.Unlock()

	response, err := conn.SubscribeEventsPlain("ALL")

	if err != nil {
		t.Errorf("SubscribeEventsPlain returned unexpected error: %v", err)
	}

	if response == nil {
		t.Fatal("SubscribeEventsPlain returned nil response")
	}

	written := buf.String()
	if !strings.Contains(written, "event plain ALL") {
		t.Errorf("Expected command to contain 'event plain ALL', got: %s", written)
	}
}

// TestSubscribeEventsJSON tests subscribing to JSON format events
func TestSubscribeEventsJSON(t *testing.T) {
	conn := NewConnection("127.0.0.1:8021", "testpass")

	var buf bytes.Buffer
	conn.mu.Lock()
	conn.connected = true
	conn.reader = bufio.NewReader(strings.NewReader("+OK\r\n\r\n"))
	conn.writer = bufio.NewWriter(&buf)
	conn.mu.Unlock()

	response, err := conn.SubscribeEventsJSON("ALL")

	if err != nil {
		t.Errorf("SubscribeEventsJSON returned unexpected error: %v", err)
	}

	if response == nil {
		t.Fatal("SubscribeEventsJSON returned nil response")
	}

	written := buf.String()
	if !strings.Contains(written, "event json ALL") {
		t.Errorf("Expected command to contain 'event json ALL', got: %s", written)
	}
}

// TestReadEventWithStopSignal tests reading events when connection is closed
func TestReadEventWithStopSignal(t *testing.T) {
	conn := NewConnection("127.0.0.1:8021", "testpass")
	close(conn.stopChan)

	event, err := conn.ReadEvent()

	if err != ErrConnectionClosed {
		t.Errorf("Expected ErrConnectionClosed, got %v", err)
	}

	if event != nil {
		t.Error("Expected event to be nil")
	}
}

// TestReadEventWithContextTimeout tests reading events with context timeout
func TestReadEventWithContextTimeout(t *testing.T) {
	conn := NewConnection("127.0.0.1:8021", "testpass")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	_, err := conn.ReadEventWithContext(ctx)

	if err != context.DeadlineExceeded {
		t.Errorf("Expected context.DeadlineExceeded, got %v", err)
	}
}

// TestReadEventWithContextCancellation tests reading events with context cancellation
func TestReadEventWithContextCancellation(t *testing.T) {
	conn := NewConnection("127.0.0.1:8021", "testpass")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := conn.ReadEventWithContext(ctx)

	if err != context.Canceled {
		t.Errorf("Expected context.Canceled, got %v", err)
	}
}

// TestTrimNewline tests the trimNewline utility function
func TestTrimNewline(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"test\r\n", "test"},
		{"test\n", "test"},
		{"test\r", "test"},
		{"test", "test"},
		{"\r\n", ""},
		{"\n", ""},
		{"\r", ""},
		{"", ""},
	}

	for _, test := range tests {
		result := trimNewline(test.input)
		if result != test.expected {
			t.Errorf("trimNewline(%q) = %q, expected %q", test.input, result, test.expected)
		}
	}
}

// TestDefaultLogger tests the default logger implementation
func TestDefaultLogger(t *testing.T) {
	logger := &DefaultLogger{}

	logger.Debug("test")
	logger.Info("test")
	logger.Warn("test")
	logger.Error("test")

	logger.Debug("test %s", "arg")
	logger.Info("test %s", "arg")
	logger.Warn("test %s", "arg")
	logger.Error("test %s", "arg")
}

// TestConnectionDoubleConnect tests that connecting an already connected connection is safe
func TestConnectionDoubleConnect(t *testing.T) {
	conn := NewConnection("127.0.0.1:8021", "testpass")

	if conn.IsConnected() {
		t.Error("Expected IsConnected to return false initially")
	}

	conn.mu.Lock()
	conn.connected = true
	conn.mu.Unlock()

	if !conn.IsConnected() {
		t.Error("Expected IsConnected to return true after setting")
	}

	ctx := context.Background()
	err := conn.Connect(ctx)
	if err != nil {
		t.Errorf("Second Connect should not fail: %v", err)
	}

	if !conn.IsConnected() {
		t.Error("Expected IsConnected to remain true")
	}
}

// TestConnectTimeout tests connection timeout behavior
func TestConnectTimeout(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("Failed to create listener: %v", err)
		return
	}
	defer listener.Close()

	conn := NewConnection(listener.Addr().String(), "testpass")
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	listener.Close()

	err = conn.Connect(ctx)
	if err == nil {
		t.Error("Expected error when connecting to closed listener")
	}
}

// TestConnectContextCancellation tests connecting with cancelled context
func TestConnectContextCancellation(t *testing.T) {
	conn := NewConnection("127.0.0.1:9999", "testpass")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := conn.Connect(ctx)
	if err == nil {
		t.Error("Expected error when context is cancelled")
	}

	if !errors.Is(err, context.Canceled) {
		t.Errorf("Expected context.Canceled error, got %v", err)
	}
}

// TestSubscribeEventsWithEmptyList tests subscribing with no event list (defaults to ALL)
func TestSubscribeEventsWithEmptyList(t *testing.T) {
	conn := NewConnection("127.0.0.1:8021", "testpass")

	var buf bytes.Buffer
	conn.mu.Lock()
	conn.connected = true
	conn.reader = bufio.NewReader(strings.NewReader("+OK\r\n\r\n"))
	conn.writer = bufio.NewWriter(&buf)
	conn.mu.Unlock()

	response, err := conn.SubscribeEvents("plain")

	if err != nil {
		t.Errorf("SubscribeEvents returned unexpected error: %v", err)
	}

	if response == nil {
		t.Fatal("SubscribeEvents returned nil response")
	}

	written := buf.String()
	if !strings.Contains(written, "event plain ALL") {
		t.Errorf("Expected command to contain 'event plain ALL', got: %s", written)
	}
}

// TestSubscribeEventsWithMultipleEvents tests subscribing to multiple specific events
func TestSubscribeEventsWithMultipleEvents(t *testing.T) {
	conn := NewConnection("127.0.0.1:8021", "testpass")

	var buf bytes.Buffer
	conn.mu.Lock()
	conn.connected = true
	conn.reader = bufio.NewReader(strings.NewReader("+OK\r\n\r\n"))
	conn.writer = bufio.NewWriter(&buf)
	conn.mu.Unlock()

	response, err := conn.SubscribeEvents("plain", "CUSTOM", "PRESENCE_IN")

	if err != nil {
		t.Errorf("SubscribeEvents returned unexpected error: %v", err)
	}

	if response == nil {
		t.Fatal("SubscribeEvents returned nil response")
	}

	written := buf.String()
	if !strings.Contains(written, "event plain CUSTOM PRESENCE_IN") {
		t.Errorf("Expected command to contain 'event plain CUSTOM PRESENCE_IN', got: %s", written)
	}
}

// TestEventChannelAndEventHandler tests that events are received correctly via the channel
func TestEventChannelAndEventHandler(t *testing.T) {
	conn := NewConnection("127.0.0.1:8021", "testpass")

	handlerCalled := false

	conn.SetEventHandler(func(event *Event) {
		handlerCalled = true
	})

	testEvent := &Event{
		Header: map[string]string{
			"Event-Name": "TEST",
		},
	}

	go func() {
		time.Sleep(10 * time.Millisecond)
		conn.eventChan <- testEvent
	}()

	select {
	case event := <-conn.EventChannel():
		if handlerCalled {
			t.Error("Event handler should not be called for direct channel send")
		}
		if event == nil {
			t.Error("Received nil event")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Timed out waiting for event")
	}
}

// TestErrorChannelReceive tests that errors are received correctly via the error channel
func TestErrorChannelReceive(t *testing.T) {
	conn := NewConnection("127.0.0.1:8021", "testpass")

	testErr := ErrAuthenticationFailed

	go func() {
		time.Sleep(10 * time.Millisecond)
		conn.errorChan <- testErr
	}()

	select {
	case err := <-conn.ErrorChannel():
		if err != testErr {
			t.Errorf("Expected error %v, got %v", testErr, err)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Timed out waiting for error")
	}
}

// TestOnDisconnectCallback tests that the disconnect callback is invoked when connection is closed
func TestOnDisconnectCallback(t *testing.T) {
	conn := NewConnection("127.0.0.1:8021", "testpass")

	callbackCalled := false

	conn.SetOnDisconnect(func() {
		callbackCalled = true
	})

	conn.mu.Lock()
	conn.connected = true
	conn.mu.Unlock()

	conn.handleDisconnect()

	time.Sleep(10 * time.Millisecond)

	if !callbackCalled {
		t.Error("On disconnect callback was not called")
	}

	if conn.IsConnected() {
		t.Error("Expected connection to be disconnected")
	}
}

// TestOnDisconnectNotCalledWhenNotConnected tests that disconnect callback is not called when not connected
func TestOnDisconnectNotCalledWhenNotConnected(t *testing.T) {
	conn := NewConnection("127.0.0.1:8021", "testpass")

	callbackCalled := false

	conn.SetOnDisconnect(func() {
		callbackCalled = true
	})

	conn.handleDisconnect()

	if callbackCalled {
		t.Error("On disconnect callback should not be called when not connected")
	}
}

// TestSendCommandFailure tests handling of failed commands
func TestSendCommandFailure(t *testing.T) {
	conn := NewConnection("127.0.0.1:8021", "testpass")

	var buf bytes.Buffer
	conn.mu.Lock()
	conn.connected = true
	conn.reader = bufio.NewReader(strings.NewReader("-ERR Invalid command\r\n\r\n"))
	conn.writer = bufio.NewWriter(&buf)
	conn.mu.Unlock()

	response, err := conn.SendCommand("invalid")

	if err != nil {
		t.Errorf("SendCommand returned unexpected error: %v", err)
	}

	if response == nil {
		t.Fatal("SendCommand returned nil response")
	}

	if response.OK {
		t.Error("Expected response.OK to be false")
	}

	if response.Reply == "" {
		t.Error("Expected response.Reply to contain error message")
	}
}

// TestLoggerInterface tests that the logger interface methods are called correctly
func TestLoggerInterface(t *testing.T) {
	logger := &testLogger{}

	logger.Debug("debug message")
	if !logger.debugCalled {
		t.Error("Debug method was not called")
	}

	logger.Info("info message")
	if !logger.infoCalled {
		t.Error("Info method was not called")
	}

	logger.Warn("warn message")
	if !logger.warnCalled {
		t.Error("Warn method was not called")
	}

	logger.Error("error message")
	if !logger.errorCalled {
		t.Error("Error method was not called")
	}
}

type mockReadWriteCloser struct {
	io.Reader
	io.Writer
}

func (m *mockReadWriteCloser) Close() error {
	return nil
}

// TestEventChannelReadWrite tests reading and writing events via the event channel
func TestEventChannelReadWrite(t *testing.T) {
	conn := NewConnection("127.0.0.1:8021", "testpass")

	testEvent := &Event{
		Header: map[string]string{
			"Event-Name": "TEST_EVENT",
		},
	}

	go func() {
		time.Sleep(10 * time.Millisecond)
		conn.eventChan <- testEvent
	}()

	select {
	case event := <-conn.EventChannel():
		if event.Header["Event-Name"] != "TEST_EVENT" {
			t.Errorf("Expected Event-Name TEST_EVENT, got %s", event.Header["Event-Name"])
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Timed out waiting for event")
	}
}

// TestConnectionCloseSafety tests that closing a connection with readers/writers is safe
func TestConnectionCloseSafety(t *testing.T) {
	conn := NewConnection("127.0.0.1:8021", "testpass")

	conn.mu.Lock()
	conn.connected = true
	conn.reader = bufio.NewReader(&bytes.Buffer{})
	conn.writer = bufio.NewWriter(&bytes.Buffer{})
	conn.mu.Unlock()

	err := conn.Close()
	if err != nil {
		t.Errorf("Close returned error: %v", err)
	}

	err = conn.Close()
	if err != nil {
		t.Errorf("Second Close returned error: %v", err)
	}
}
