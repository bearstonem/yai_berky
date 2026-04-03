package main

import (
	"fmt"
	"log"
	"os"

	"github.com/ekkinox/yai/ui"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	input, err := ui.NewUIInput()
	if err != nil {
		log.Fatal(err)
	}

	if input.IsPipeMode() {
		if err := ui.RunPipe(input); err != nil {
			fmt.Fprintf(os.Stderr, "error: %s\n", err)
			os.Exit(1)
		}
		return
	}

	if _, err := tea.NewProgram(ui.NewUi(input)).Run(); err != nil {
		log.Fatal(err)
	}
}
