package prompt

import (
	"fmt"

	"github.com/aaronflorey/bin/pkg/spinner"
	"github.com/charmbracelet/huh"
)

type MultiSelectOption struct {
	Label string
	Value string
}

func MultiSelect(title string, options []MultiSelectOption) ([]string, error) {
	if len(options) == 0 {
		return nil, nil
	}

	resume := spinner.Pause()
	defer resume()

	selected := make([]string, 0, len(options))
	huhOptions := make([]huh.Option[string], 0, len(options))
	for _, option := range options {
		huhOptions = append(huhOptions, huh.NewOption(option.Label, option.Value))
	}

	field := huh.NewMultiSelect[string]().
		Title(title).
		Description("Use arrows to move, space to toggle, and enter to confirm").
		Options(huhOptions...).
		Value(&selected)

	if err := huh.NewForm(huh.NewGroup(field)).Run(); err != nil {
		return nil, fmt.Errorf("command aborted")
	}

	return selected, nil
}
