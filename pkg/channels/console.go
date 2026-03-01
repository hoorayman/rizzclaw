package channels

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/hoorayman/rizzclaw/pkg/bus"
)

// ConsoleChannel implements a console-based channel for CLI interaction
type ConsoleChannel struct {
	*BaseChannel
	reader      *bufio.Reader
	writer      io.Writer
	startChan   chan struct{}
	startOnce   sync.Once
	needPrompt  bool
	mu          sync.Mutex
}

// NewConsoleChannel creates a new console channel
func NewConsoleChannel(bus *bus.MessageBus) *ConsoleChannel {
	return &ConsoleChannel{
		BaseChannel: NewBaseChannel("console", bus, nil),
		reader:      bufio.NewReader(os.Stdin),
		writer:      os.Stdout,
		startChan:   make(chan struct{}),
	}
}

// Start starts the console channel
func (c *ConsoleChannel) Start(ctx context.Context) error {
	c.setRunning(true)
	go c.run(ctx)
	return nil
}

// PrintBanner prints the console channel banner
func (c *ConsoleChannel) PrintBanner() {
	fmt.Fprintln(c.writer, "🦞 RizzClaw Gateway Console Mode")
	fmt.Fprintln(c.writer, "Type your message and press Enter. Type '/exit' to quit.")
}

// SignalStart signals that all initialization is done and input loop can start
func (c *ConsoleChannel) SignalStart() {
	c.startOnce.Do(func() {
		close(c.startChan)
	})
}

// run runs the console input loop
func (c *ConsoleChannel) run(ctx context.Context) {
	// Wait for SignalStart before printing first prompt
	<-c.startChan

	// Print first prompt
	c.printPrompt()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		input, err := c.reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				fmt.Fprintln(c.writer, "\nGoodbye!")
				return
			}
			fmt.Fprintf(c.writer, "Error reading input: %v\n", err)
			c.printPrompt()
			continue
		}

		input = strings.TrimSpace(input)
		if input == "" {
			c.printPrompt()
			continue
		}

		if input == "/exit" || input == "/quit" {
			fmt.Fprintln(c.writer, "Goodbye!")
			return
		}

		// Mark that we need a prompt after response
		c.mu.Lock()
		c.needPrompt = true
		c.mu.Unlock()

		// Publish message to bus
		c.HandleMessage("console_user", "console_chat", input, nil)
	}
}

// printPrompt prints the user prompt
func (c *ConsoleChannel) printPrompt() {
	fmt.Fprint(c.writer, "😎: ")
}

// Stop stops the console channel
func (c *ConsoleChannel) Stop(ctx context.Context) error {
	c.setRunning(false)
	return nil
}

// Send sends a message to the console
func (c *ConsoleChannel) Send(ctx context.Context, msg bus.OutboundMessage) error {
	c.mu.Lock()
	needPrompt := c.needPrompt
	c.needPrompt = false
	c.mu.Unlock()

	fmt.Fprintf(c.writer, "🦞: %s\n", msg.Content)

	if needPrompt {
		c.printPrompt()
	}
	return nil
}
