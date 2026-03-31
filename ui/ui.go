package ui

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/ekkinox/yai/ai"
	"github.com/ekkinox/yai/config"
	"github.com/ekkinox/yai/history"
	"github.com/ekkinox/yai/run"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/spf13/viper"
)

type configStep int

const (
	configStepProvider configStep = iota
	configStepAPIKey
	configStepBaseURL
	configStepDone
)

type UiState struct {
	error       error
	runMode     RunMode
	promptMode  PromptMode
	configuring bool
	querying    bool
	confirming  bool
	executing   bool
	args        string
	pipe        string
	buffer      string
	command     string
	// multi-step config wizard state
	configStep     configStep
	configProvider string
	configKey      string
	configBaseURL  string
	// agent mode state
	agentRunning         bool
	agentApprovalPending bool
	// setup flag
	forceSetup bool
	// remote SSH target
	remoteHost string
}

type UiDimensions struct {
	width  int
	height int
}

type UiComponents struct {
	prompt   *Prompt
	renderer *Renderer
	spinner  *Spinner
}

type Ui struct {
	state      UiState
	dimensions UiDimensions
	components UiComponents
	config     *config.Config
	engine     *ai.Engine
	history    *history.History
}

func NewUi(input *UiInput) *Ui {
	prompt := NewPrompt(input.GetPromptMode())
	if input.GetRemote() != "" {
		prompt.SetRemoteHost(input.GetRemote())
	}

	return &Ui{
		state: UiState{
			error:       nil,
			runMode:     input.GetRunMode(),
			promptMode:  input.GetPromptMode(),
			configuring: false,
			querying:    false,
			confirming:  false,
			executing:   false,
			args:        input.GetArgs(),
			pipe:        input.GetPipe(),
			buffer:      "",
			command:     "",
			configStep:  configStepProvider,
			forceSetup:  input.IsSetup(),
			remoteHost:  input.GetRemote(),
		},
		dimensions: UiDimensions{
			150,
			150,
		},
		components: UiComponents{
			prompt:   prompt,
			renderer: NewRenderer(
				glamour.WithAutoStyle(),
				glamour.WithWordWrap(150),
			),
			spinner: NewSpinner(),
		},
		history: history.NewHistory(),
	}
}

func (u *Ui) Init() tea.Cmd {
	if u.state.forceSetup {
		return tea.Sequence(
			tea.ClearScreen,
			u.startConfig(),
		)
	}

	config, err := config.NewConfig()
	if err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			if u.state.runMode == ReplMode {
				return tea.Sequence(
					tea.ClearScreen,
					u.startConfig(),
				)
			} else {
				return u.startConfig()
			}
		} else {
			return tea.Sequence(
				tea.Println(u.components.renderer.RenderError(err.Error())),
				tea.Quit,
			)
		}
	}

	if u.state.runMode == ReplMode {
		return u.startRepl(config)
	} else {
		return u.startCli(config)
	}
}

func (u *Ui) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var (
		cmds       []tea.Cmd
		promptCmd  tea.Cmd
		spinnerCmd tea.Cmd
	)

	switch msg := msg.(type) {
	// spinner
	case spinner.TickMsg:
		if u.state.querying {
			u.components.spinner, spinnerCmd = u.components.spinner.Update(msg)
			cmds = append(cmds, spinnerCmd)
		}
	// size
	case tea.WindowSizeMsg:
		u.dimensions.width = msg.Width
		u.dimensions.height = msg.Height
		u.components.renderer = NewRenderer(
			glamour.WithAutoStyle(),
			glamour.WithWordWrap(u.dimensions.width),
		)
	// keyboard
	case tea.KeyMsg:
		switch msg.Type {
		// quit / interrupt
		case tea.KeyCtrlC:
			if u.state.agentRunning {
				u.engine.Interrupt()
				u.state.agentRunning = false
				u.state.agentApprovalPending = false
				u.state.querying = false
				u.components.prompt.Focus()
				return u, tea.Sequence(
					tea.Println(u.components.renderer.RenderWarning("\n[agent interrupted]\n")),
					textinput.Blink,
				)
			}
			return u, tea.Quit
		// history
		case tea.KeyUp, tea.KeyDown:
			if !u.state.querying && !u.state.confirming && !u.state.configuring {
				var input *string
				if msg.Type == tea.KeyUp {
					input = u.history.GetPrevious()
				} else {
					input = u.history.GetNext()
				}
				if input != nil {
					u.components.prompt.SetValue(*input)
					u.components.prompt, promptCmd = u.components.prompt.Update(msg)
					cmds = append(cmds, promptCmd)
				}
			}
		// switch mode
		case tea.KeyTab:
			if !u.state.querying && !u.state.confirming && !u.state.configuring && !u.state.agentRunning {
				switch u.state.promptMode {
				case ExecPromptMode:
					u.state.promptMode = ChatPromptMode
					u.components.prompt.SetMode(ChatPromptMode)
					u.engine.SetMode(ai.ChatEngineMode)
				case ChatPromptMode:
					u.state.promptMode = AgentPromptMode
					u.components.prompt.SetMode(AgentPromptMode)
					u.engine.SetMode(ai.AgentEngineMode)
				default:
					u.state.promptMode = ExecPromptMode
					u.components.prompt.SetMode(ExecPromptMode)
					u.engine.SetMode(ai.ExecEngineMode)
				}
				u.engine.Reset()
				u.components.prompt, promptCmd = u.components.prompt.Update(msg)
				cmds = append(cmds, promptCmd, textinput.Blink)
			}
		// enter
		case tea.KeyEnter:
			if u.state.configuring {
				return u, u.handleConfigInput(u.components.prompt.GetValue())
			}
			if !u.state.querying && !u.state.confirming && !u.state.agentRunning {
				input := u.components.prompt.GetValue()
				if input != "" {
					inputPrint := u.components.prompt.AsString()
					u.history.Add(input)
					u.components.prompt.SetValue("")
					u.components.prompt.Blur()
					u.components.prompt, promptCmd = u.components.prompt.Update(msg)
					if u.state.promptMode == AgentPromptMode {
						cmds = append(
							cmds,
							promptCmd,
							tea.Println(inputPrint),
							u.startAgent(input),
							u.awaitAgentEvent(),
						)
					} else if u.state.promptMode == ChatPromptMode {
						cmds = append(
							cmds,
							promptCmd,
							tea.Println(inputPrint),
							u.startChatStream(input),
							u.awaitChatStream(),
						)
					} else {
						cmds = append(
							cmds,
							promptCmd,
							tea.Println(inputPrint),
							u.startExec(input),
							u.components.spinner.Tick,
						)
					}
				}
			}

		// help
		case tea.KeyCtrlH:
			if !u.state.configuring && !u.state.querying && !u.state.confirming {
				u.components.prompt, promptCmd = u.components.prompt.Update(msg)
				cmds = append(
					cmds,
					promptCmd,
					tea.Println(u.components.renderer.RenderContent(u.components.renderer.RenderHelpMessage())),
					textinput.Blink,
				)
			}

		// clear
		case tea.KeyCtrlL:
			if !u.state.querying && !u.state.confirming {
				u.components.prompt, promptCmd = u.components.prompt.Update(msg)
				cmds = append(cmds, promptCmd, tea.ClearScreen, textinput.Blink)
			}

		// reset
		case tea.KeyCtrlR:
			if !u.state.querying && !u.state.confirming {
				u.history.Reset()
				u.engine.Reset()
				u.components.prompt.SetValue("")
				u.components.prompt, promptCmd = u.components.prompt.Update(msg)
				cmds = append(cmds, promptCmd, tea.ClearScreen, textinput.Blink)
			}

		// edit settings
		case tea.KeyCtrlS:
			if !u.state.querying && !u.state.confirming && !u.state.configuring && !u.state.executing {
				u.state.executing = true
				u.state.buffer = ""
				u.state.command = ""
				u.components.prompt.Blur()
				u.components.prompt, promptCmd = u.components.prompt.Update(msg)
				cmds = append(cmds, promptCmd, u.editSettings())
			}

		default:
			if u.state.agentApprovalPending {
				if strings.ToLower(msg.String()) == "y" {
					u.state.agentApprovalPending = false
					u.engine.SendApproval(true)
					return u, u.awaitAgentEvent()
				} else {
					u.state.agentApprovalPending = false
					u.engine.SendApproval(false)
					return u, tea.Sequence(
						tea.Println(u.components.renderer.RenderWarning("  [skipped]")),
						u.awaitAgentEvent(),
					)
				}
			} else if u.state.confirming {
				if strings.ToLower(msg.String()) == "y" {
					u.state.confirming = false
					u.state.executing = true
					u.state.buffer = ""
					u.components.prompt.SetValue("")
					return u, tea.Sequence(
						promptCmd,
						u.execCommand(u.state.command),
					)
				} else {
					u.state.confirming = false
					u.state.executing = false
					u.state.buffer = ""
					u.components.prompt, promptCmd = u.components.prompt.Update(msg)
					u.components.prompt.SetValue("")
					u.components.prompt.Focus()
					if u.state.runMode == ReplMode {
						cmds = append(
							cmds,
							promptCmd,
							tea.Println(fmt.Sprintf("\n%s\n", u.components.renderer.RenderWarning("[cancel]"))),
							textinput.Blink,
						)
					} else {
						return u, tea.Sequence(
							promptCmd,
							tea.Println(fmt.Sprintf("\n%s\n", u.components.renderer.RenderWarning("[cancel]"))),
							tea.Quit,
						)
					}
				}
				u.state.command = ""
			} else {
				u.components.prompt.Focus()
				u.components.prompt, promptCmd = u.components.prompt.Update(msg)
				cmds = append(cmds, promptCmd, textinput.Blink)
			}
		}
	// engine exec feedback
	case ai.EngineExecOutput:
		var output string
		if msg.IsExecutable() {
			u.state.confirming = true
			u.state.command = msg.GetCommand()
			output = u.components.renderer.RenderContent(fmt.Sprintf("`%s`", u.state.command))
			if run.CommandContainsSudo(u.state.command) {
				output += fmt.Sprintf("  %s\n", u.components.renderer.RenderWarning("[sudo] this command requires elevated privileges"))
			}
			output += fmt.Sprintf("  %s\n\n  confirm execution? [y/N]", u.components.renderer.RenderHelp(msg.GetExplanation()))
			u.components.prompt.Blur()
		} else {
			output = u.components.renderer.RenderContent(msg.GetExplanation())
			u.components.prompt.Focus()
			if u.state.runMode == CliMode {
				return u, tea.Sequence(
					tea.Println(output),
					tea.Quit,
				)
			}
		}
		u.components.prompt, promptCmd = u.components.prompt.Update(msg)
		return u, tea.Sequence(
			promptCmd,
			textinput.Blink,
			tea.Println(output),
		)
	// engine chat stream feedback
	case ai.EngineChatStreamOutput:
		if msg.IsLast() {
			output := u.components.renderer.RenderContent(u.state.buffer)
			u.state.buffer = ""
			u.components.prompt.Focus()
			if u.state.runMode == CliMode {
				return u, tea.Sequence(
					tea.Println(output),
					tea.Quit,
				)
			} else {
				return u, tea.Sequence(
					tea.Println(output),
					textinput.Blink,
				)
			}
		} else {
			return u, u.awaitChatStream()
		}
	// agent event feedback
	case ai.AgentEvent:
		switch msg.Type {
		case ai.AgentEventThinking:
			return u, tea.Sequence(
				tea.Println(u.components.renderer.RenderAgentThinking(msg.Content)),
				u.awaitAgentEvent(),
			)
		case ai.AgentEventToolCall:
			tc := msg.ToolCall
			output := u.components.renderer.RenderToolCall(tc.Name, tc.Arguments)
			return u, tea.Sequence(
				tea.Println(output),
				u.awaitAgentEvent(),
			)
		case ai.AgentEventApprovalRequired:
			u.state.agentApprovalPending = true
			tc := msg.ToolCall
			prompt := fmt.Sprintf("  Run %s? [y/N]", u.components.renderer.RenderContent(fmt.Sprintf("`%s`", formatToolCallSummary(tc))))
			return u, tea.Println(prompt)
		case ai.AgentEventToolResult:
			tr := msg.ToolResult
			exitCode := 0
			if strings.Contains(tr.Content, "exit_code: ") {
				fmt.Sscanf(tr.Content, "exit_code: %d", &exitCode)
			}
			output := u.components.renderer.RenderToolResult(tr.Content, exitCode)
			return u, tea.Sequence(
				tea.Println(output),
				u.awaitAgentEvent(),
			)
		case ai.AgentEventAnswer:
			output := u.components.renderer.RenderContent(msg.Content)
			return u, tea.Sequence(
				tea.Println(output),
				u.awaitAgentEvent(),
			)
		case ai.AgentEventError:
			errStr := "unknown error"
			if msg.Error != nil {
				errStr = msg.Error.Error()
			}
			return u, tea.Sequence(
				tea.Println(u.components.renderer.RenderError(fmt.Sprintf("[agent error] %s", errStr))),
				u.awaitAgentEvent(),
			)
		case ai.AgentEventDone:
			u.state.agentRunning = false
			u.state.querying = false
			u.components.prompt.Focus()
			if u.state.runMode == CliMode {
				return u, tea.Quit
			}
			return u, textinput.Blink
		}
	// runner feedback
	case run.RunOutput:
		u.state.querying = false
		u.components.prompt, promptCmd = u.components.prompt.Update(msg)
		u.components.prompt.Focus()
		output := u.components.renderer.RenderSuccess(fmt.Sprintf("\n%s\n", msg.GetSuccessMessage()))
		if msg.HasError() {
			output = u.components.renderer.RenderError(fmt.Sprintf("\n%s\n", msg.GetErrorMessage()))
		}
		if u.state.runMode == CliMode {
			return u, tea.Sequence(
				tea.Println(output),
				tea.Quit,
			)
		} else {
			return u, tea.Sequence(
				tea.Println(output),
				promptCmd,
				textinput.Blink,
			)
		}
	// errors
	case error:
		u.state.error = msg
		return u, nil
	}

	return u, tea.Batch(cmds...)
}

func (u *Ui) View() string {
	if u.state.error != nil {
		return u.components.renderer.RenderError(fmt.Sprintf("[error] %s", u.state.error))
	}

	if u.state.configuring {
		return fmt.Sprintf(
			"%s\n%s",
			u.components.renderer.RenderContent(u.state.buffer),
			u.components.prompt.View(),
		)
	}

	if u.state.agentApprovalPending {
		return ""
	}

	if !u.state.querying && !u.state.confirming && !u.state.executing && !u.state.agentRunning {
		return u.components.prompt.View()
	}

	if u.state.agentRunning {
		return u.components.spinner.View()
	}

	if u.state.promptMode == ChatPromptMode {
		return u.components.renderer.RenderContent(u.state.buffer)
	} else {
		if u.state.querying {
			return u.components.spinner.View()
		} else {
			if !u.state.executing {
				return u.components.renderer.RenderContent(u.state.buffer)
			}
		}
	}

	return ""
}

func (u *Ui) startRepl(cfg *config.Config) tea.Cmd {
	return tea.Sequence(
		tea.ClearScreen,
		tea.Println(u.components.renderer.RenderContent(u.components.renderer.RenderHelpMessage())),
		textinput.Blink,
		func() tea.Msg {
			u.config = cfg

			if u.state.promptMode == DefaultPromptMode {
				u.state.promptMode = GetPromptModeFromString(cfg.GetUserConfig().GetDefaultPromptMode())
			}

			engineMode := promptModeToEngineMode(u.state.promptMode)

			engine, err := ai.NewEngine(engineMode, cfg)
			if err != nil {
				return err
			}

			if u.state.pipe != "" {
				engine.SetPipe(u.state.pipe)
			}

			if u.state.remoteHost != "" {
				if err := engine.SetRemoteHost(u.state.remoteHost); err != nil {
					return err
				}
			}

			u.engine = engine

			providerInfo := u.components.renderer.RenderProviderInfo(cfg.GetAiConfig())
			u.state.buffer = fmt.Sprintf("%s\n\n", providerInfo)
			if u.state.remoteHost != "" && engine.GetRemoteInfo() != nil {
				info := engine.GetRemoteInfo()
				remoteInfo := u.components.renderer.RenderRemoteInfo(u.state.remoteHost, info.Hostname, info.OS)
				u.state.buffer += fmt.Sprintf("%s\n\n", remoteInfo)
			}
			u.state.command = ""
			u.components.prompt = NewPrompt(u.state.promptMode)
			if u.state.remoteHost != "" {
				u.components.prompt.SetRemoteHost(u.state.remoteHost)
			}

			return nil
		},
	)
}

func (u *Ui) startCli(cfg *config.Config) tea.Cmd {
	u.config = cfg

	if u.state.promptMode == DefaultPromptMode {
		u.state.promptMode = GetPromptModeFromString(cfg.GetUserConfig().GetDefaultPromptMode())
	}

	engineMode := promptModeToEngineMode(u.state.promptMode)

	engine, err := ai.NewEngine(engineMode, cfg)
	if err != nil {
		u.state.error = err
		return nil
	}

	if u.state.pipe != "" {
		engine.SetPipe(u.state.pipe)
	}

	if u.state.remoteHost != "" {
		if err := engine.SetRemoteHost(u.state.remoteHost); err != nil {
			u.state.error = err
			return nil
		}
	}

	u.engine = engine
	u.state.querying = true
	u.state.confirming = false
	u.state.buffer = ""
	u.state.command = ""

	switch u.state.promptMode {
	case AgentPromptMode:
		var bannerCmd tea.Cmd
		if u.state.remoteHost != "" && engine.GetRemoteInfo() != nil {
			info := engine.GetRemoteInfo()
			remoteInfo := u.components.renderer.RenderRemoteInfo(u.state.remoteHost, info.Hostname, info.OS)
			bannerCmd = tea.Println(u.components.renderer.RenderContent(remoteInfo))
		}
		if bannerCmd != nil {
			return tea.Sequence(
				bannerCmd,
				tea.Batch(
					u.startAgent(u.state.args),
					u.awaitAgentEvent(),
				),
			)
		}
		return tea.Batch(
			u.startAgent(u.state.args),
			u.awaitAgentEvent(),
		)
	case ExecPromptMode:
		return tea.Batch(
			u.components.spinner.Tick,
			func() tea.Msg {
				output, err := u.engine.ExecCompletion(u.state.args)
				u.state.querying = false
				if err != nil {
					return err
				}
				return *output
			},
		)
	default:
		return tea.Batch(
			u.startChatStream(u.state.args),
			u.awaitChatStream(),
		)
	}
}

func (u *Ui) startConfig() tea.Cmd {
	return func() tea.Msg {
		u.state.configuring = true
		u.state.querying = false
		u.state.confirming = false
		u.state.executing = false
		u.state.configStep = configStepProvider

		u.state.buffer = u.components.renderer.RenderConfigMessage()
		u.state.command = ""
		u.components.prompt = NewPrompt(ConfigPromptMode)
		u.components.prompt.SetEchoMode(textinput.EchoNormal)
		u.components.prompt.SetPlaceholder("Enter provider number (1-8)...")

		return nil
	}
}

func (u *Ui) handleConfigInput(input string) tea.Cmd {
	input = strings.TrimSpace(input)

	switch u.state.configStep {
	case configStepProvider:
		return u.handleProviderSelection(input)
	case configStepAPIKey:
		return u.handleAPIKeyInput(input)
	case configStepBaseURL:
		return u.handleBaseURLInput(input)
	default:
		return nil
	}
}

func (u *Ui) handleProviderSelection(input string) tea.Cmd {
	providers := config.ProviderList()
	num, err := strconv.Atoi(input)
	if err != nil || num < 1 || num > len(providers) {
		return func() tea.Msg {
			u.state.buffer = u.components.renderer.RenderConfigMessage()
			u.state.buffer += u.components.renderer.RenderError(
				fmt.Sprintf("\nInvalid selection. Please enter a number between 1 and %d.\n", len(providers)),
			)
			u.components.prompt.SetValue("")
			return nil
		}
	}

	u.state.configProvider = providers[num-1]
	u.components.prompt.SetValue("")

	if config.ProviderNeedsAPIKey(u.state.configProvider) {
		u.state.configStep = configStepAPIKey
		return func() tea.Msg {
			u.state.buffer = u.components.renderer.RenderAPIKeyMessage(u.state.configProvider)
			u.components.prompt.SetEchoMode(textinput.EchoPassword)
			u.components.prompt.SetPlaceholder("Enter your API key...")
			return nil
		}
	}

	if u.state.configProvider == config.ProviderCustom {
		u.state.configStep = configStepBaseURL
		return func() tea.Msg {
			u.state.buffer = u.components.renderer.RenderBaseURLMessage(u.state.configProvider)
			u.components.prompt.SetEchoMode(textinput.EchoNormal)
			u.components.prompt.SetPlaceholder("https://your-server.com/v1")
			return nil
		}
	}

	u.state.configKey = ""
	return u.finishConfig()
}

func (u *Ui) handleAPIKeyInput(input string) tea.Cmd {
	if input == "" {
		return func() tea.Msg {
			u.state.buffer = u.components.renderer.RenderAPIKeyMessage(u.state.configProvider)
			u.state.buffer += u.components.renderer.RenderError("\nAPI key cannot be empty.\n")
			u.components.prompt.SetValue("")
			return nil
		}
	}

	u.state.configKey = input
	u.components.prompt.SetValue("")

	if u.state.configProvider == config.ProviderCustom {
		u.state.configStep = configStepBaseURL
		return func() tea.Msg {
			u.state.buffer = u.components.renderer.RenderBaseURLMessage(u.state.configProvider)
			u.components.prompt.SetEchoMode(textinput.EchoNormal)
			u.components.prompt.SetPlaceholder("https://your-server.com/v1")
			return nil
		}
	}

	return u.finishConfig()
}

func (u *Ui) handleBaseURLInput(input string) tea.Cmd {
	u.state.configBaseURL = input
	u.components.prompt.SetValue("")
	return u.finishConfig()
}

func (u *Ui) finishConfig() tea.Cmd {
	u.state.configuring = false
	u.state.configStep = configStepDone

	cfg, err := config.WriteConfig(
		u.state.configProvider,
		u.state.configKey,
		"",
		u.state.configBaseURL,
		true,
	)
	if err != nil {
		u.state.error = err
		return nil
	}

	u.config = cfg
	engine, err := ai.NewEngine(ai.ExecEngineMode, cfg)
	if err != nil {
		u.state.error = err
		return nil
	}

	if u.state.pipe != "" {
		engine.SetPipe(u.state.pipe)
	}

	u.engine = engine

	providerInfo := u.components.renderer.RenderProviderInfo(cfg.GetAiConfig())

	if u.state.runMode == ReplMode {
		return tea.Sequence(
			tea.ClearScreen,
			tea.Println(u.components.renderer.RenderSuccess("\n[settings ok]\n")),
			tea.Println(u.components.renderer.RenderContent(providerInfo)),
			textinput.Blink,
			func() tea.Msg {
				u.state.buffer = ""
				u.state.command = ""
				u.components.prompt = NewPrompt(ExecPromptMode)
				return nil
			},
		)
	} else {
		u.state.querying = true
		u.state.configuring = false
		u.state.buffer = ""
		switch u.state.promptMode {
		case AgentPromptMode:
			return tea.Batch(
				tea.Println(u.components.renderer.RenderSuccess("\n[settings ok]")),
				u.startAgent(u.state.args),
				u.awaitAgentEvent(),
			)
		case ChatPromptMode:
			return tea.Batch(
				tea.Println(u.components.renderer.RenderSuccess("\n[settings ok]")),
				u.startChatStream(u.state.args),
				u.awaitChatStream(),
			)
		default:
			return tea.Sequence(
				tea.Println(u.components.renderer.RenderSuccess("\n[settings ok]")),
				u.components.spinner.Tick,
				func() tea.Msg {
					output, err := u.engine.ExecCompletion(u.state.args)
					u.state.querying = false
					if err != nil {
						return err
					}
					return *output
				},
			)
		}
	}
}

func (u *Ui) startExec(input string) tea.Cmd {
	return func() tea.Msg {
		u.state.querying = true
		u.state.confirming = false
		u.state.buffer = ""
		u.state.command = ""

		output, err := u.engine.ExecCompletion(input)
		u.state.querying = false
		if err != nil {
			return err
		}

		return *output
	}
}

func (u *Ui) startChatStream(input string) tea.Cmd {
	return func() tea.Msg {
		u.state.querying = true
		u.state.executing = false
		u.state.confirming = false
		u.state.buffer = ""
		u.state.command = ""

		err := u.engine.ChatStreamCompletion(input)
		if err != nil {
			return err
		}

		return nil
	}
}

func (u *Ui) awaitChatStream() tea.Cmd {
	return func() tea.Msg {
		output := <-u.engine.GetChannel()
		u.state.buffer += output.GetContent()
		u.state.querying = !output.IsLast()

		return output
	}
}

func (u *Ui) startAgent(input string) tea.Cmd {
	return func() tea.Msg {
		u.state.querying = true
		u.state.agentRunning = true
		u.state.executing = false
		u.state.confirming = false
		u.state.buffer = ""
		u.state.command = ""

		autoExec := false
		if u.config != nil {
			autoExec = u.config.GetUserConfig().GetAgentAutoExecute()
		}

		err := u.engine.AgentCompletion(input, autoExec)
		if err != nil {
			return err
		}

		return nil
	}
}

func (u *Ui) awaitAgentEvent() tea.Cmd {
	return func() tea.Msg {
		event := <-u.engine.GetAgentChannel()
		return event
	}
}

func promptModeToEngineMode(pm PromptMode) ai.EngineMode {
	switch pm {
	case ChatPromptMode:
		return ai.ChatEngineMode
	case AgentPromptMode:
		return ai.AgentEngineMode
	default:
		return ai.ExecEngineMode
	}
}

func formatToolCallSummary(tc *ai.ToolCall) string {
	if tc == nil {
		return ""
	}
	switch tc.Name {
	case "run_command":
		var args struct {
			Command string `json:"command"`
		}
		if err := json.Unmarshal([]byte(tc.Arguments), &args); err == nil {
			return args.Command
		}
	case "read_file", "list_directory":
		var args struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal([]byte(tc.Arguments), &args); err == nil {
			return fmt.Sprintf("%s %s", tc.Name, args.Path)
		}
	case "write_file":
		var args struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal([]byte(tc.Arguments), &args); err == nil {
			return fmt.Sprintf("write_file %s", args.Path)
		}
	}
	return fmt.Sprintf("%s(%s)", tc.Name, tc.Arguments)
}

func (u *Ui) execCommand(input string) tea.Cmd {
	u.state.querying = false
	u.state.confirming = false
	u.state.executing = true

	var c *exec.Cmd
	if run.CommandContainsSudo(input) {
		c = run.PrepareSudoInteractiveCommand(input)
	} else {
		c = run.PrepareInteractiveCommand(input)
	}

	return tea.ExecProcess(c, func(error error) tea.Msg {
		u.state.executing = false
		u.state.command = ""

		return run.NewRunOutput(error, "[error]", "[ok]")
	})
}

func (u *Ui) editSettings() tea.Cmd {
	u.state.querying = false
	u.state.confirming = false
	u.state.executing = true

	c := run.PrepareEditSettingsCommand(fmt.Sprintf(
		"%s %s",
		u.config.GetSystemConfig().GetEditor(),
		u.config.GetSystemConfig().GetConfigFile(),
	))

	return tea.ExecProcess(c, func(error error) tea.Msg {
		u.state.executing = false
		u.state.command = ""

		if error != nil {
			return run.NewRunOutput(error, "[settings error]", "")
		}

		cfg, error := config.NewConfig()
		if error != nil {
			return run.NewRunOutput(error, "[settings error]", "")
		}

		u.config = cfg
		engine, error := ai.NewEngine(ai.ExecEngineMode, cfg)
		if u.state.pipe != "" {
			engine.SetPipe(u.state.pipe)
		}
		if error != nil {
			return run.NewRunOutput(error, "[settings error]", "")
		}
		u.engine = engine

		return run.NewRunOutput(nil, "", "[settings ok]")
	})
}
