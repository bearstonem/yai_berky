package command

import (
	"fmt"
	"sync"
)

// Pricing per 1M tokens (input, output) in USD.
var modelPricing = map[string][2]float64{
	// OpenAI
	"gpt-4o":          {2.50, 10.00},
	"gpt-4o-mini":     {0.15, 0.60},
	"gpt-4-turbo":     {10.00, 30.00},
	"gpt-4":           {30.00, 60.00},
	"gpt-3.5-turbo":   {0.50, 1.50},
	"o1":              {15.00, 60.00},
	"o1-mini":         {3.00, 12.00},
	// Anthropic
	"claude-opus-4-6":       {15.00, 75.00},
	"claude-sonnet-4-6":     {3.00, 15.00},
	"claude-haiku-4-5":      {0.80, 4.00},
	"claude-3-5-sonnet":     {3.00, 15.00},
	"claude-3-opus":         {15.00, 75.00},
	"claude-3-sonnet":       {3.00, 15.00},
	"claude-3-haiku":        {0.25, 1.25},
}

type UsageTracker struct {
	mu           sync.Mutex
	model        string
	inputTokens  int
	outputTokens int
	requests     int
}

func NewUsageTracker(model string) *UsageTracker {
	return &UsageTracker{model: model}
}

func (u *UsageTracker) SetModel(model string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.model = model
}

func (u *UsageTracker) Add(inputTokens, outputTokens int) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.inputTokens += inputTokens
	u.outputTokens += outputTokens
	u.requests++
}

func (u *UsageTracker) Cost() float64 {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.cost()
}

func (u *UsageTracker) cost() float64 {
	pricing, ok := modelPricing[u.model]
	if !ok {
		// Try prefix matching for versioned model names
		for prefix, p := range modelPricing {
			if len(u.model) > len(prefix) && u.model[:len(prefix)] == prefix {
				pricing = p
				ok = true
				break
			}
		}
	}
	if !ok {
		return 0
	}
	inputCost := float64(u.inputTokens) / 1_000_000 * pricing[0]
	outputCost := float64(u.outputTokens) / 1_000_000 * pricing[1]
	return inputCost + outputCost
}

func (u *UsageTracker) Summary() string {
	u.mu.Lock()
	defer u.mu.Unlock()

	total := u.inputTokens + u.outputTokens
	cost := u.cost()

	summary := fmt.Sprintf("**Session Usage**\n\n"+
		"| Metric | Value |\n"+
		"|---|---|\n"+
		"| Requests | %d |\n"+
		"| Input tokens | %d |\n"+
		"| Output tokens | %d |\n"+
		"| Total tokens | %d |\n"+
		"| Model | `%s` |\n",
		u.requests, u.inputTokens, u.outputTokens, total, u.model)

	if cost > 0 {
		summary += fmt.Sprintf("| Est. cost | $%.4f |\n", cost)
	} else {
		summary += "| Est. cost | unknown (no pricing for model) |\n"
	}

	return summary
}

func (u *UsageTracker) Reset() {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.inputTokens = 0
	u.outputTokens = 0
	u.requests = 0
}
