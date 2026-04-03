package command

import "strings"

// Result tells the UI what to do after a command executes.
type Result struct {
	Output  string // text to display (markdown)
	IsError bool
	Quit    bool // signal the TUI to quit
	Clear   bool // clear the screen
	Reset   bool // reset conversation history
}

// Handler executes a slash command and returns its result.
type Handler func(args string, ctx *Context) Result

// Command describes a single slash command.
type Command struct {
	Name        string
	Aliases     []string
	Description string
	Handler     Handler
}

// Registry holds all registered commands.
type Registry struct {
	commands map[string]*Command
	ordered  []*Command // insertion order for /help listing
}

func NewRegistry() *Registry {
	return &Registry{
		commands: make(map[string]*Command),
	}
}

func (r *Registry) Register(cmd *Command) {
	r.commands[cmd.Name] = cmd
	r.ordered = append(r.ordered, cmd)
	for _, alias := range cmd.Aliases {
		r.commands[alias] = cmd
	}
}

func (r *Registry) Get(name string) *Command {
	return r.commands[name]
}

func (r *Registry) All() []*Command {
	return r.ordered
}

// Parse splits user input into command name and arguments.
// Returns empty name if input is not a slash command.
func Parse(input string) (name string, args string) {
	input = strings.TrimSpace(input)
	if !strings.HasPrefix(input, "/") {
		return "", ""
	}
	input = input[1:] // strip leading /
	parts := strings.SplitN(input, " ", 2)
	name = strings.ToLower(parts[0])
	if len(parts) > 1 {
		args = strings.TrimSpace(parts[1])
	}
	return name, args
}
