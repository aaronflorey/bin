package prompt

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/aaronflorey/bin/pkg/spinner"
	"golang.org/x/term"
)

var stdin io.Reader = os.Stdin

func IsInteractive() bool {
	f, ok := stdin.(*os.File)
	if !ok {
		return true
	}
	return term.IsTerminal(int(f.Fd()))
}

// Confirm prints a confirmation prompt
// for the given message and waits for the
// users input.
func Confirm(message string) error {
	resume := spinner.Pause()
	defer resume()

	fmt.Printf("\n%s [Y/n] ", message)
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
	case "", "y", "yes":
	default:
		return fmt.Errorf("command aborted")
	}

	return nil
}
