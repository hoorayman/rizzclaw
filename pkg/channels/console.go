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

type ConsoleChannel struct {
	*BaseChannel
	reader        *bufio.Reader
	writer        io.Writer
	startChan     chan struct{}
	startOnce     sync.Once
	needPrompt    bool
	mu            sync.Mutex
	clearCallback func() error
}

func NewConsoleChannel(bus *bus.MessageBus) *ConsoleChannel {
	return &ConsoleChannel{
		BaseChannel: NewBaseChannel("console", bus, nil),
		reader:      bufio.NewReader(os.Stdin),
		writer:      os.Stdout,
		startChan:   make(chan struct{}),
	}
}

func (c *ConsoleChannel) Start(ctx context.Context) error {
	c.setRunning(true)
	go c.run(ctx)
	return nil
}

func (c *ConsoleChannel) PrintBanner() {
	fmt.Fprintln(c.writer, "🦞 RizzClaw Gateway Console Mode")
	fmt.Fprintln(c.writer, "Type your message and press Enter. Commands: /exit, /clear")
}

func (c *ConsoleChannel) SetClearCallback(cb func() error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.clearCallback = cb
}

func (c *ConsoleChannel) SignalStart() {
	c.startOnce.Do(func() {
		close(c.startChan)
	})
}

func (c *ConsoleChannel) run(ctx context.Context) {
	<-c.startChan

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

		if input == "/clear" {
			c.mu.Lock()
			cb := c.clearCallback
			c.mu.Unlock()

			if cb != nil {
				if err := cb(); err != nil {
					fmt.Fprintf(c.writer, "❌ Failed to clear session: %v\n", err)
				} else {
					fmt.Fprintln(c.writer, "✅ Session cleared successfully!")
				}
			} else {
				fmt.Fprintln(c.writer, "⚠️ Clear callback not set")
			}
			c.printPrompt()
			continue
		}

		c.mu.Lock()
		c.needPrompt = true
		c.mu.Unlock()

		c.HandleMessage("console_user", "console_chat", input, nil)
	}
}

func (c *ConsoleChannel) printPrompt() {
	fmt.Fprint(c.writer, "😎: ")
}

func (c *ConsoleChannel) Stop(ctx context.Context) error {
	c.setRunning(false)
	return nil
}

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
