package cmd

import (
	"fmt"
	"os"
	"sync"
	"time"
)

var spinnerFrames = []rune{'\u280b', '\u2819', '\u2839', '\u2838', '\u283c', '\u2834', '\u2826', '\u2827', '\u2807', '\u280f'}

// spinner is a simple terminal progress indicator written to stderr (never
// stdout, so it can't corrupt --json output or a piped report). It's purely
// cosmetic \u2014 it reflects what the agent already decided, never influences it.
type spinner struct {
	mu      sync.Mutex
	label   string
	stopCh  chan struct{}
	doneCh  chan struct{}
	running bool
}

func newSpinner() *spinner {
	return &spinner{}
}

// Start begins (or retargets) the animated line. Safe to call repeatedly.
func (s *spinner) Start(label string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.label = label
	if s.running {
		return
	}
	s.running = true
	s.stopCh = make(chan struct{})
	s.doneCh = make(chan struct{})

	go func() {
		defer close(s.doneCh)
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		i := 0
		for {
			select {
			case <-s.stopCh:
				return
			case <-ticker.C:
				s.mu.Lock()
				label := s.label
				s.mu.Unlock()
				fmt.Fprintf(os.Stderr, "\r%c %s\033[K", spinnerFrames[i%len(spinnerFrames)], label)
				i++
			}
		}
	}()
}

// Retarget updates the label of an already-running spinner without a restart.
func (s *spinner) Retarget(label string) {
	s.mu.Lock()
	s.label = label
	s.mu.Unlock()
}

// Stop halts the animation and clears the line, optionally printing a final
// one-line status (e.g. the tool call that just completed) before returning.
func (s *spinner) Stop(final string) {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.running = false
	close(s.stopCh)
	s.mu.Unlock()
	<-s.doneCh

	fmt.Fprintf(os.Stderr, "\r\033[K")
	if final != "" {
		fmt.Fprintln(os.Stderr, final)
	}
}

// formatToolCall renders a short, single-line summary of a dispatched tool
// call for the spinner's status line, truncating long argument JSON.
func formatToolCall(toolName, argsJSON string) string {
	const maxArgsLen = 60
	args := argsJSON
	if len(args) > maxArgsLen {
		args = args[:maxArgsLen] + "..."
	}
	if args == "" || args == "{}" {
		return toolName + "()"
	}
	return fmt.Sprintf("%s(%s)", toolName, args)
}
