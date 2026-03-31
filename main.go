package main

import (
	"log"

	"github.com/ekkinox/yai/ui"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	input, err := ui.NewUIInput()
	if err != nil {
		log.Fatal(err)
	}

	if _, err := tea.NewProgram(ui.NewUi(input)).Run(); err != nil {
		log.Fatal(err)
	}
}
