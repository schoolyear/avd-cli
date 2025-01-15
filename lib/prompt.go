package lib

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/friendsofgo/errors"
)

func PromptUserInput(prompt string) (string, error) {
	if prompt != "" {
		fmt.Printf("%s", prompt)
	}
	val, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return "", errors.Wrap(err, "failed to read from Stdin")
	}

	return strings.TrimSpace(val), nil
}
