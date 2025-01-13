package lib

import (
	"github.com/c-bata/go-prompt"
	"os"
)

func PromptOptionCtrlCExit() prompt.Option {
	return prompt.OptionAddKeyBind(prompt.KeyBind{
		Key: prompt.ControlC,
		Fn: func(buffer *prompt.Buffer) {
			os.Exit(1)
		},
	})
}

func PromptNoCompletions() prompt.Completer {
	return func(document prompt.Document) []prompt.Suggest { return nil }
}
