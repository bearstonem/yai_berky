package main

import (
	"fmt"
	"log"
	"os"

	"github.com/bearstonem/helm/ai"
	"github.com/bearstonem/helm/config"
	"github.com/bearstonem/helm/ui"
	"github.com/bearstonem/helm/web"
	"github.com/mitchellh/go-homedir"

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

	if input.IsGUIMode() {
		if err := startGUI(input); err != nil {
			fmt.Fprintf(os.Stderr, "error: %s\n", err)
			os.Exit(1)
		}
		return
	}

	if _, err := tea.NewProgram(ui.NewUi(input)).Run(); err != nil {
		log.Fatal(err)
	}
}

func startGUI(input *ui.UiInput) error {
	cfg, err := config.NewConfig()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	engine, err := ai.NewEngine(ai.AgentEngineMode, cfg)
	if err != nil {
		return fmt.Errorf("creating engine: %w", err)
	}

	homeDir, err := homedir.Dir()
	if err != nil {
		return fmt.Errorf("finding home dir: %w", err)
	}

	srv := web.NewServer(cfg, engine, homeDir, input.GetGUIPort())
	return srv.Start()
}
