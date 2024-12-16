package textio

import (
	"fmt"
	"strings"
)

var chars = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

type SpinnerPrinter struct {
	lastContent string
	idx         int
	chars       []string
	running     bool
}

func NewSpinnerPrinter() *SpinnerPrinter {
	return &SpinnerPrinter{
		chars: chars,
	}
}

func (s *SpinnerPrinter) EnsureNewline() {
	if !strings.HasSuffix(s.lastContent, "\n") {
		s.Print("\n")
	}
}

func (s *SpinnerPrinter) Start() {
	if !s.running {
		// Hide the cursor
		fmt.Print("\033[?25l")
		s.running = true
	}
}

func (s *SpinnerPrinter) Stop() {
	if s.running {
		// Clear spinner
		fmt.Println("\b ")
		// Show the cursor
		fmt.Print("\033[?25h")
		s.running = false
		// Overwrite spinner char
	}
}

func (s *SpinnerPrinter) Tick() {
	s.Print("")
}

func (s *SpinnerPrinter) Print(content string) {
	if !s.running {
		panic("spinner not running")
	}
	spinnerChar := s.chars[s.idx]
	s.idx = (s.idx + 1) % len(s.chars)

	if content == "" {
		fmt.Print("\b" + spinnerChar)
	} else {
		prefix := "\b"
		if strings.HasPrefix(content, "\n") {
			prefix = "\b "
		}
		s.lastContent = content
		fmt.Print(prefix + content + spinnerChar)
	}
}
