package command

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParse(t *testing.T) {
	tests := []struct {
		input string
		name  string
		args  string
	}{
		{"/help", "help", ""},
		{"/model gpt-4", "model", "gpt-4"},
		{"/skill remove myskill", "skill", "remove myskill"},
		{"/agent select code_reviewer", "agent", "select code_reviewer"},
		{"  /clear  ", "clear", ""},
		{"/commit fix the bug", "commit", "fix the bug"},
	}

	for _, tt := range tests {
		name, args := Parse(tt.input)
		assert.Equal(t, tt.name, name, "input: %q", tt.input)
		assert.Equal(t, tt.args, args, "input: %q", tt.input)
	}
}

func TestRegistry(t *testing.T) {
	r := NewRegistry()

	r.Register(&Command{
		Name:        "test",
		Aliases:     []string{"t"},
		Description: "A test command",
		Handler:     func(_ string, _ *Context) Result { return Result{Output: "ok"} },
	})

	t.Run("get by name", func(t *testing.T) {
		cmd := r.Get("test")
		assert.NotNil(t, cmd)
		assert.Equal(t, "test", cmd.Name)
	})

	t.Run("get by alias", func(t *testing.T) {
		cmd := r.Get("t")
		assert.NotNil(t, cmd)
		assert.Equal(t, "test", cmd.Name)
	})

	t.Run("get unknown", func(t *testing.T) {
		cmd := r.Get("unknown")
		assert.Nil(t, cmd)
	})

	t.Run("execute", func(t *testing.T) {
		cmd := r.Get("test")
		result := cmd.Handler("", nil)
		assert.Equal(t, "ok", result.Output)
	})
}

func TestRegisterBuiltins(t *testing.T) {
	r := NewRegistry()
	RegisterBuiltins(r)

	expected := []string{
		"help", "clear", "reset", "compact", "cost",
		"session", "mode", "model", "yolo",
		"diff", "commit", "status", "log",
		"memory", "integrate", "skill",
		"agent", "goals",
	}

	for _, name := range expected {
		cmd := r.Get(name)
		assert.NotNil(t, cmd, "command %q not registered", name)
	}
}

func TestCmdHelp(t *testing.T) {
	result := cmdHelp("", &Context{})
	assert.Contains(t, result.Output, "/help")
	assert.Contains(t, result.Output, "/agent")
	assert.Contains(t, result.Output, "/goals")
	assert.Contains(t, result.Output, "/skill")
	assert.False(t, result.IsError)
}

func TestCmdClear(t *testing.T) {
	result := cmdClear("", nil)
	assert.True(t, result.Clear)
}

func TestCmdMode(t *testing.T) {
	result := cmdMode("", &Context{Mode: "agent"})
	assert.Contains(t, result.Output, "agent")

	result = cmdMode("chat", &Context{Mode: "agent"})
	assert.Contains(t, result.Output, "switch:chat")
}

func TestCmdModeInvalid(t *testing.T) {
	result := cmdMode("invalid", &Context{})
	assert.True(t, result.IsError)
}

func TestCmdGoalsNoCallback(t *testing.T) {
	result := cmdGoals("", &Context{})
	assert.True(t, result.IsError)
}

func TestCmdAgentNoCallback(t *testing.T) {
	result := cmdAgent("", &Context{})
	assert.True(t, result.IsError)
}

func TestCmdSessionNoCallback(t *testing.T) {
	result := cmdSession("", &Context{})
	assert.True(t, result.IsError)
}

func TestCmdSessionLoad(t *testing.T) {
	loaded := ""
	ctx := &Context{
		LoadSessionFn: func(id string) error {
			loaded = id
			return nil
		},
	}

	result := cmdSession("load abc123", ctx)
	assert.False(t, result.IsError)
	assert.Equal(t, "abc123", loaded)
}
