package ui

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/bearstonem/helm/config"
)

const (
	colorReset  = "\033[0m"
	colorBold   = "\033[1m"
	colorDim    = "\033[2m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorCyan   = "\033[36m"
	colorPurple = "\033[35m"
)

// RunSetupWizard runs an interactive first-time setup experience.
// Returns true if the user wants to launch the GUI afterward.
func RunSetupWizard() bool {
	reader := bufio.NewReader(os.Stdin)

	printBanner()
	printSection("Welcome to Helm")
	fmt.Println("  Helm is an AI agent platform for your terminal. It manages autonomous")
	fmt.Println("  agents, creates reusable tools, remembers across sessions, and provides")
	fmt.Println("  a web-based dashboard for managing everything.")
	fmt.Println()
	printFeatures()

	pressEnter(reader)

	// Step 1: Provider selection
	printSection("Step 1: Choose an AI Provider")
	providers := config.ProviderList()
	for i, id := range providers {
		name := config.ProviderDisplayNames[id]
		model := config.ProviderDefaultModels[id]
		needsKey := "requires API key"
		if !config.ProviderNeedsAPIKey(id) {
			needsKey = colorGreen + "no API key needed (local)" + colorReset
		}
		fmt.Printf("  %s%d%s. %-25s %-20s %s\n", colorCyan, i+1, colorReset, name, colorDim+"("+model+")"+colorReset, needsKey)
	}
	fmt.Println()

	provider := ""
	for {
		fmt.Printf("  %sSelect provider [1-%d]:%s ", colorYellow, len(providers), colorReset)
		input := readLine(reader)
		num, err := strconv.Atoi(input)
		if err == nil && num >= 1 && num <= len(providers) {
			provider = providers[num-1]
			break
		}
		fmt.Println("  Invalid selection. Try again.")
	}
	fmt.Printf("  %s✓%s Selected: %s\n\n", colorGreen, colorReset, config.ProviderDisplayNames[provider])

	// Step 2: API Key
	apiKey := ""
	if config.ProviderNeedsAPIKey(provider) {
		printSection("Step 2: API Key")
		envVar := config.ProviderEnvKeys[provider]
		if envVar != "" {
			fmt.Printf("  You can also set this as %s%s%s environment variable.\n\n", colorCyan, envVar, colorReset)
		}
		fmt.Printf("  %sEnter your API key:%s ", colorYellow, colorReset)
		apiKey = readLine(reader)
		if apiKey != "" {
			fmt.Printf("  %s✓%s API key saved\n\n", colorGreen, colorReset)
		} else {
			fmt.Printf("  %s!%s Skipped — you can set it later in ~/.config/helm.json or via env var\n\n", colorYellow, colorReset)
		}
	} else {
		printSection("Step 2: API Key")
		fmt.Printf("  %s✓%s %s doesn't require an API key — it runs locally!\n\n", colorGreen, colorReset, config.ProviderDisplayNames[provider])
	}

	// Step 3: Model
	defaultModel := config.ProviderDefaultModels[provider]
	printSection("Step 3: Model")
	fmt.Printf("  Default model: %s%s%s\n", colorCyan, defaultModel, colorReset)
	fmt.Printf("  %sPress Enter to keep default, or type a model name:%s ", colorYellow, colorReset)
	modelInput := readLine(reader)
	model := defaultModel
	if modelInput != "" {
		model = modelInput
	}
	fmt.Printf("  %s✓%s Using model: %s\n\n", colorGreen, colorReset, model)

	// Step 4: Base URL
	baseURL := ""
	providerURL := config.ProviderBaseURLs[provider]
	if provider == config.ProviderCustom || provider == config.ProviderOllama || provider == config.ProviderLlamaCpp || provider == config.ProviderLMStudio {
		printSection("Step 4: Base URL")
		if providerURL != "" {
			fmt.Printf("  Default: %s%s%s\n", colorCyan, providerURL, colorReset)
		}
		fmt.Printf("  %sPress Enter for default, or enter custom URL:%s ", colorYellow, colorReset)
		urlInput := readLine(reader)
		if urlInput != "" {
			baseURL = urlInput
		}
		fmt.Printf("  %s✓%s Base URL: %s\n\n", colorGreen, colorReset, orDefault(baseURL, "(provider default)"))
	}

	// Step 5: Preferences
	printSection("Step 5: Preferences (optional)")
	fmt.Println("  Add any natural-language preferences for how Helm should behave.")
	fmt.Println("  Example: \"Always explain commands before running. Prefer Python over Bash.\"")
	fmt.Printf("  %sPreferences (or press Enter to skip):%s ", colorYellow, colorReset)
	preferences := readLine(reader)
	if preferences != "" {
		fmt.Printf("  %s✓%s Preferences saved\n\n", colorGreen, colorReset)
	} else {
		fmt.Printf("  %s✓%s No preferences — you can add them later\n\n", colorGreen, colorReset)
	}

	// Step 6: Permission mode
	printSection("Step 6: Permission Mode")
	fmt.Println("  Controls what the agent is allowed to do:")
	fmt.Printf("  %s1%s. read-only      — can only read files and search\n", colorCyan, colorReset)
	fmt.Printf("  %s2%s. workspace-write — can read, write, run commands, create skills %s(recommended)%s\n", colorCyan, colorReset, colorGreen, colorReset)
	fmt.Printf("  %s3%s. full-access     — everything including restart_helm\n", colorCyan, colorReset)
	fmt.Println()
	permMode := "workspace-write"
	fmt.Printf("  %sSelect [1-3, default 2]:%s ", colorYellow, colorReset)
	permInput := readLine(reader)
	switch permInput {
	case "1":
		permMode = "read-only"
	case "3":
		permMode = "full-access"
	}
	fmt.Printf("  %s✓%s Permission mode: %s\n\n", colorGreen, colorReset, permMode)

	// Save configuration
	printSection("Saving Configuration")

	_, err := config.WriteConfig(provider, apiKey, model, baseURL, true)
	if err != nil {
		fmt.Printf("  %s✗ Error saving config: %s%s\n", colorYellow, err, colorReset)
		fmt.Println("  You may need to create ~/.config/ directory first.")
	} else {
		// Set additional settings
		if preferences != "" {
			setViperKey("USER_PREFERENCES", preferences)
		}
		if permMode != "workspace-write" {
			setViperKey("USER_PERMISSION_MODE", permMode)
		}
		fmt.Printf("  %s✓%s Configuration saved to ~/.config/helm.json\n\n", colorGreen, colorReset)
	}

	// Summary
	printSection("Setup Complete!")
	fmt.Println()
	fmt.Printf("  %sHow to use Helm:%s\n\n", colorBold, colorReset)
	fmt.Printf("    %shelm%s                   Interactive TUI (terminal)\n", colorCyan, colorReset)
	fmt.Printf("    %shelm --gui%s             Web dashboard (agents, skills, themes)\n", colorCyan, colorReset)
	fmt.Printf("    %shelm -a%s <task>         Agent mode — autonomous multi-step tasks\n", colorCyan, colorReset)
	fmt.Printf("    %shelm -c%s <question>     Chat mode — ask anything\n", colorCyan, colorReset)
	fmt.Printf("    %shelm -e%s <query>        Exec mode — generate a shell command\n", colorCyan, colorReset)
	fmt.Printf("    %shelm --pipe -a%s <task>  Headless mode — for scripts and CI\n", colorCyan, colorReset)
	fmt.Println()
	fmt.Printf("  %sInside the TUI:%s press %stab%s to switch modes, %sctrl+h%s for help\n\n", colorBold, colorReset, colorCyan, colorReset, colorCyan, colorReset)

	// Offer to launch GUI
	fmt.Printf("  %sWould you like to launch the web GUI now? [Y/n]%s ", colorYellow, colorReset)
	answer := strings.ToLower(strings.TrimSpace(readLine(reader)))
	fmt.Println()

	return answer == "" || answer == "y" || answer == "yes"
}

func printBanner() {
	fmt.Println()
	fmt.Printf("  %s%s", colorPurple, colorBold)
	fmt.Println("    ╦ ╦ ╔═╗ ╦   ╔╦╗")
	fmt.Println("    ╠═╣ ║╣  ║   ║║║")
	fmt.Println("    ╩ ╩ ╚═╝ ╩═╝ ╩ ╩")
	fmt.Printf("  %s%s  AI Agent Platform%s\n\n", colorReset, colorDim, colorReset)
}

func printSection(title string) {
	fmt.Printf("  %s%s━━ %s ━━%s\n\n", colorPurple, colorBold, title, colorReset)
}

func printFeatures() {
	features := []struct{ icon, name, desc string }{
		{"▶", "Exec Mode", "Generate shell commands from natural language"},
		{"📡", "Chat Mode", "Ask questions, get markdown answers"},
		{"🖖", "Agent Mode", "Autonomous multi-step task execution with tools"},
		{"🛸", "Sub-Agents", "Create specialized agents that collaborate"},
		{"⚡", "Skills", "Self-creating reusable tools (bash, python, node)"},
		{"🔄", "Self-Improve", "Autonomous evolution loop with goals"},
		{"🌐", "Web GUI", "Full dashboard with themes and visualizations"},
		{"🔍", "Web Search", "Built-in Brave Search (set BRAVE_API_KEY)"},
	}

	for _, f := range features {
		fmt.Printf("    %s  %s%-15s%s %s\n", f.icon, colorBold, f.name, colorReset, f.desc)
	}
	fmt.Println()
}

func pressEnter(reader *bufio.Reader) {
	fmt.Printf("  %sPress Enter to continue...%s", colorDim, colorReset)
	reader.ReadString('\n')
	fmt.Println()
}

func readLine(reader *bufio.Reader) string {
	line, _ := reader.ReadString('\n')
	return strings.TrimSpace(line)
}

func orDefault(val, def string) string {
	if val == "" {
		return def
	}
	return val
}

func setViperKey(key, value string) {
	// Import viper indirectly to avoid circular deps — use WriteConfig
	// The config is already loaded by WriteConfig, just need to set + save
	// For simplicity, we modify the file directly
	// This is handled by the existing config.SaveAllSettings
	config.SaveAllSettings(map[string]interface{}{key: value})
}
