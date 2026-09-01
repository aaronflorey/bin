package prompt

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/aaronflorey/bin/pkg/spinner"
	"golang.org/x/term"
)

type MultiSelectOption struct {
	Label string
	Value string
}

var stdin io.Reader = os.Stdin

func IsInteractive() bool {
	f, ok := stdin.(*os.File)
	if !ok {
		return true
	}
	return term.IsTerminal(int(f.Fd()))
}

func SelectMultiple(msg string, options []MultiSelectOption) ([]string, error) {
	if len(options) == 0 {
		return nil, nil
	}
	if !IsInteractive() {
		return nil, fmt.Errorf("interactive selection required")
	}

	resume := spinner.Pause()
	defer resume()

	fmt.Printf("\n%s\n\n", msg)
	for i, opt := range options {
		fmt.Printf("  [%d] %s\n", i+1, opt.Label)
	}
	fmt.Printf("\nEnter comma-separated numbers (e.g. 1,3,5) or 'all': ")

	reader := bufio.NewReader(stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		if err == io.EOF {
			return nil, fmt.Errorf("command aborted")
		}
		return nil, fmt.Errorf("invalid input")
	}

	input = strings.TrimSpace(strings.ToLower(input))
	if input == "" {
		return nil, fmt.Errorf("command aborted")
	}

	if input == "all" {
		result := make([]string, len(options))
		for i, opt := range options {
			result[i] = opt.Value
		}
		return result, nil
	}

	selected := []string{}
	parts := strings.Split(input, ",")
	seen := map[int]bool{}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		idx, err := strconv.Atoi(part)
		if err != nil || idx < 1 || idx > len(options) {
			return nil, fmt.Errorf("invalid selection: %s", part)
		}
		if seen[idx] {
			continue
		}
		seen[idx] = true
		selected = append(selected, options[idx-1].Value)
	}

	return selected, nil
}

// Confirm prints a confirmation prompt
// for the given message and waits for the
// users input.
func Confirm(message string) error {
	return confirm(message, true)
}

// ConfirmDefaultNo asks for explicit confirmation.
func ConfirmDefaultNo(message string) error {
	return confirm(message, false)
}

func confirm(message string, defaultYes bool) error {
	resume := spinner.Pause()
	defer resume()

	choice := "[y/N]"
	if defaultYes {
		choice = "[Y/n]"
	}
	fmt.Printf("\n%s %s ", message, choice)
	reader := bufio.NewReader(stdin)
	var response string

	response, err := reader.ReadString('\n')
	if err != nil {
		if err == io.EOF {
			return fmt.Errorf("command aborted")
		}
		return fmt.Errorf("invalid input")
	}

	switch strings.ToLower(strings.TrimSpace(response)) {
	case "y", "yes":
		return nil
	case "":
		if defaultYes {
			return nil
		}
		return fmt.Errorf("command aborted")
	default:
		return fmt.Errorf("command aborted")
	}
}
