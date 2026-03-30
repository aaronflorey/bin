package prompt

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/aaronflorey/bin/pkg/spinner"
)

var stdin io.Reader = os.Stdin

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
		if errors.Is(err, io.EOF) {
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
