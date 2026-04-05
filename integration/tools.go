package integration

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/bearstonem/helm/config"
)

// ToolDef mirrors ai.Tool without importing the ai package (to avoid cycles).
type ToolDef struct {
	Name        string
	Description string
	Parameters  json.RawMessage
}

// ToolResult is the output of executing an integration tool.
type ToolResult struct {
	Content string
}

// IntegrationTool wraps a configured integration as an executable agent tool.
type IntegrationTool struct {
	Def    ToolDef
	Config config.IntegrationConfig
}

// BuildTools creates agent-ready tool definitions from enabled integrations.
func BuildTools(integrations []config.IntegrationConfig) []IntegrationTool {
	var tools []IntegrationTool
	for _, ic := range integrations {
		if !ic.Enabled {
			continue
		}
		switch ic.Type {
		case config.IntegrationComfyUI:
			tools = append(tools, buildComfyUITool(ic))
		case config.IntegrationWebhook:
			tools = append(tools, buildWebhookTool(ic))
		case config.IntegrationMCP:
			mcpTools, err := BuildMCPToolDefs(ic)
			if err != nil {
				// Log error but don't fail the entire build
				continue
			}
			for _, mt := range mcpTools {
				tools = append(tools, IntegrationTool{
					Def:    mt,
					Config: ic,
				})
			}
		}
	}
	return tools
}

func buildComfyUITool(ic config.IntegrationConfig) IntegrationTool {
	name := sanitizeToolName(ic.Name)
	desc := fmt.Sprintf("Generate an image using ComfyUI (%s). Provide a text prompt describing the desired image. Optionally provide a negative prompt.", ic.Name)

	schema := json.RawMessage(`{
		"type": "object",
		"properties": {
			"prompt": {
				"type": "string",
				"description": "Text description of the image to generate (positive prompt)"
			},
			"negative_prompt": {
				"type": "string",
				"description": "What to avoid in the image (negative prompt). Optional."
			},
			"output_dir": {
				"type": "string",
				"description": "Directory to save the output image. Defaults to current directory."
			}
		},
		"required": ["prompt"]
	}`)

	return IntegrationTool{
		Def: ToolDef{
			Name:        name,
			Description: desc,
			Parameters:  schema,
		},
		Config: ic,
	}
}

func buildWebhookTool(ic config.IntegrationConfig) IntegrationTool {
	name := sanitizeToolName(ic.Name)
	desc := fmt.Sprintf("Call the webhook '%s' at %s", ic.Name, ic.Endpoint)

	schema := json.RawMessage(`{
		"type": "object",
		"properties": {
			"body": {
				"type": "string",
				"description": "JSON body to send with the request"
			}
		}
	}`)

	return IntegrationTool{
		Def: ToolDef{
			Name:        name,
			Description: desc,
			Parameters:  schema,
		},
		Config: ic,
	}
}

// Execute runs an integration tool and returns the result.
func Execute(tool IntegrationTool, argsJSON string) ToolResult {
	switch tool.Config.Type {
	case config.IntegrationComfyUI:
		return executeComfyUI(tool, argsJSON)
	case config.IntegrationWebhook:
		return executeWebhook(tool, argsJSON)
	case config.IntegrationMCP:
		return ExecuteMCPTool(tool.Config, tool.Def.Name, argsJSON)
	default:
		return ToolResult{Content: fmt.Sprintf("error: unknown integration type %s", tool.Config.Type)}
	}
}

func executeComfyUI(tool IntegrationTool, argsJSON string) ToolResult {
	var args struct {
		Prompt         string `json:"prompt"`
		NegativePrompt string `json:"negative_prompt"`
		OutputDir      string `json:"output_dir"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return ToolResult{Content: fmt.Sprintf("error parsing arguments: %s", err)}
	}

	if args.Prompt == "" {
		return ToolResult{Content: "error: prompt is required"}
	}

	outputDir := args.OutputDir
	if outputDir == "" {
		outputDir, _ = os.Getwd()
	}

	client := NewComfyUIClient(tool.Config.Endpoint)

	workflow := tool.Config.Workflow
	if len(workflow) == 0 {
		return ToolResult{Content: "error: no workflow configured for this ComfyUI integration. Use /integrate to set one up."}
	}

	modifiedWorkflow, err := InjectPromptText(workflow, args.Prompt, args.NegativePrompt)
	if err != nil {
		return ToolResult{Content: fmt.Sprintf("error modifying workflow: %s", err)}
	}

	queueResp, err := client.QueuePrompt(modifiedWorkflow)
	if err != nil {
		return ToolResult{Content: fmt.Sprintf("error submitting to ComfyUI: %s", err)}
	}

	entry, err := client.WaitForCompletion(queueResp.PromptID, 5*time.Minute)
	if err != nil {
		return ToolResult{Content: fmt.Sprintf("error waiting for ComfyUI: %s", err)}
	}

	var downloaded []string
	for _, nodeOutput := range entry.Outputs {
		for _, img := range nodeOutput.Images {
			localPath, err := client.DownloadImage(img, outputDir)
			if err != nil {
				return ToolResult{Content: fmt.Sprintf("error downloading image: %s", err)}
			}
			downloaded = append(downloaded, localPath)
		}
	}

	if len(downloaded) == 0 {
		return ToolResult{Content: "ComfyUI workflow completed but no images were produced."}
	}

	return ToolResult{
		Content: fmt.Sprintf("Generated %d image(s):\n%s", len(downloaded), strings.Join(downloaded, "\n")),
	}
}

func executeWebhook(tool IntegrationTool, argsJSON string) ToolResult {
	var args struct {
		Body string `json:"body"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return ToolResult{Content: fmt.Sprintf("error parsing arguments: %s", err)}
	}

	method := tool.Config.Method
	if method == "" {
		method = "POST"
	}

	var body io.Reader
	if args.Body != "" {
		body = strings.NewReader(args.Body)
	}

	req, err := http.NewRequest(method, tool.Config.Endpoint, body)
	if err != nil {
		return ToolResult{Content: fmt.Sprintf("error creating request: %s", err)}
	}
	req.Header.Set("Content-Type", "application/json")
	if tool.Config.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+tool.Config.APIKey)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return ToolResult{Content: fmt.Sprintf("error calling webhook: %s", err)}
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	result := string(respBody)
	if len(result) > 2000 {
		result = result[:2000] + "\n... [truncated]"
	}

	return ToolResult{Content: fmt.Sprintf("HTTP %d\n%s", resp.StatusCode, result)}
}

func sanitizeToolName(name string) string {
	name = strings.ToLower(name)
	name = strings.ReplaceAll(name, " ", "_")
	name = strings.ReplaceAll(name, "-", "_")
	return "integration_" + name
}
