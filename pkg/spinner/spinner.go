package spinner

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/term"
)

const (
	defaultMessage = "Processing..."
	renderInterval = 100 * time.Millisecond
	startDelay     = 200 * time.Millisecond
)

var frames = []string{"-", "\\", "|", "/"}

var manager struct {
	mu      sync.Mutex
	current *Spinner
}

type Spinner struct {
	message string

	mu      sync.Mutex
	running bool
	paused  bool
	done    chan struct{}
}

func Start(message string) *Spinner {
	if !isEnabled() {
		return nil
	}

	if strings.TrimSpace(message) == "" {
		message = defaultMessage
	}

	s := &Spinner{
		message: message,
		running: true,
		done:    make(chan struct{}),
	}

	manager.mu.Lock()
	manager.current = s
	manager.mu.Unlock()

	go s.loop()
	return s
}

func Stop() {
	manager.mu.Lock()
	current := manager.current
	manager.current = nil
	manager.mu.Unlock()

	if current != nil {
		current.Stop()
	}
}

func Pause() func() {
	manager.mu.Lock()
	current := manager.current
	manager.mu.Unlock()

	if current == nil {
		return func() {}
	}

	current.Pause()
	return func() {
		current.Resume()
	}
}

func Writer(w io.Writer) io.Writer {
	return stopWriter{writer: w}
}

func (s *Spinner) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.running = false
	close(s.done)
	s.mu.Unlock()

	clearLine()
}

func (s *Spinner) Pause() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running || s.paused {
		return
	}

	s.paused = true
	clearLine()
}

func (s *Spinner) Resume() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return
	}

	s.paused = false
}

func (s *Spinner) loop() {
	timer := time.NewTimer(startDelay)
	defer timer.Stop()

	select {
	case <-s.done:
		return
	case <-timer.C:
	}

	ticker := time.NewTicker(renderInterval)
	defer ticker.Stop()

	frameIdx := 0
	for {
		select {
		case <-s.done:
			return
		case <-ticker.C:
			s.mu.Lock()
			running := s.running
			paused := s.paused
			message := s.message
			s.mu.Unlock()

			if !running {
				return
			}
			if paused {
				continue
			}

			fmt.Fprintf(os.Stderr, "\r%s %s", frames[frameIdx], message)
			frameIdx = (frameIdx + 1) % len(frames)
		}
	}
}

func clearLine() {
	fmt.Fprintf(os.Stderr, "\r%s\r", strings.Repeat(" ", len(defaultMessage)+4))
}

func isEnabled() bool {
	if os.Getenv("TERM") == "dumb" {
		return false
	}

	return term.IsTerminal(int(os.Stderr.Fd()))
}

type stopWriter struct {
	writer io.Writer
}

func (w stopWriter) Write(p []byte) (int, error) {
	Stop()
	return w.writer.Write(p)
}

func (w stopWriter) Fd() uintptr {
	if writer, ok := w.writer.(interface{ Fd() uintptr }); ok {
		return writer.Fd()
	}

	return 0
}
