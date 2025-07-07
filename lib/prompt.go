package lib

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/friendsofgo/errors"
)

func PromptUserInput(prompt string, defaultValue *string) (string, error) {
	if prompt != "" {
		fmt.Printf("%s", prompt)
	}
	if defaultValue != nil {
		fmt.Printf("(Default: %s): ", *defaultValue)
	}
	val, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return "", errors.Wrap(err, "failed to read from Stdin")
	}

	out := strings.TrimSpace(val)
	if out == "" && defaultValue != nil {
		out = *defaultValue
	}

	return out, nil
}

func PromptEnum(prompt string, options []string, linePrefix string, defaultIdx *int) (int, error) {
	for i, option := range options {
		fmt.Printf("%s- [%d] %s\n", linePrefix, i+1, option)
	}

	var promptText string
	if defaultIdx == nil {
		promptText = fmt.Sprintf("%s%s: ", linePrefix, prompt)
	} else {
		promptText = fmt.Sprintf("%s%s (Default: %d): ", linePrefix, prompt, *defaultIdx+1)
	}

	selectionStr, err := PromptUserInput(promptText, nil)
	if err != nil {
		return 0, errors.Wrap(err, "failed to read the user input")
	}
	if selectionStr == "" && defaultIdx != nil {
		return *defaultIdx, nil
	}

	selection, err := strconv.Atoi(selectionStr)
	if err != nil {
		return 0, errors.Wrap(err, "invalid input")
	}

	if selection > len(options) || selection < 1 {
		return 0, fmt.Errorf("invalid selection %d", selection)
	}

	return selection - 1, nil
}
