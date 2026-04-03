package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
)

// ComfyUIClient talks to a ComfyUI server's REST API.
type ComfyUIClient struct {
	endpoint string
	clientID string
}

func NewComfyUIClient(endpoint string) *ComfyUIClient {
	return &ComfyUIClient{
		endpoint: endpoint,
		clientID: uuid.New().String(),
	}
}

// QueuePromptResponse is the response from POST /prompt.
type QueuePromptResponse struct {
	PromptID string `json:"prompt_id"`
	Number   int    `json:"number"`
}

// QueuePrompt submits a workflow (API-format prompt JSON) for execution.
// The prompt parameter should be a JSON object keyed by node IDs.
func (c *ComfyUIClient) QueuePrompt(prompt json.RawMessage) (*QueuePromptResponse, error) {
	body := map[string]interface{}{
		"prompt":    json.RawMessage(prompt),
		"client_id": c.clientID,
	}
	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal prompt: %w", err)
	}

	resp, err := http.Post(c.endpoint+"/prompt", "application/json", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("POST /prompt: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("POST /prompt returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result QueuePromptResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return &result, nil
}

// HistoryEntry represents a completed prompt's output.
type HistoryEntry struct {
	Outputs map[string]NodeOutput `json:"outputs"`
	Status  struct {
		StatusStr string `json:"status_str"`
		Completed bool   `json:"completed"`
	} `json:"status"`
}

type NodeOutput struct {
	Images []ImageRef `json:"images"`
}

type ImageRef struct {
	Filename  string `json:"filename"`
	Subfolder string `json:"subfolder"`
	Type      string `json:"type"`
}

// GetHistory polls for the prompt result. Returns nil if not yet completed.
func (c *ComfyUIClient) GetHistory(promptID string) (*HistoryEntry, error) {
	resp, err := http.Get(fmt.Sprintf("%s/history/%s", c.endpoint, promptID))
	if err != nil {
		return nil, fmt.Errorf("GET /history: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET /history returned %d", resp.StatusCode)
	}

	var result map[string]HistoryEntry
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode history: %w", err)
	}

	entry, ok := result[promptID]
	if !ok {
		return nil, nil // not completed yet
	}
	return &entry, nil
}

// DownloadImage fetches an output image and saves it to the specified directory.
// Returns the local file path.
func (c *ComfyUIClient) DownloadImage(ref ImageRef, outputDir string) (string, error) {
	url := fmt.Sprintf("%s/view?filename=%s&subfolder=%s&type=%s",
		c.endpoint, ref.Filename, ref.Subfolder, ref.Type)

	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("GET /view: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GET /view returned %d", resp.StatusCode)
	}

	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return "", fmt.Errorf("mkdir: %w", err)
	}

	localPath := filepath.Join(outputDir, ref.Filename)
	f, err := os.Create(localPath)
	if err != nil {
		return "", fmt.Errorf("create file: %w", err)
	}
	defer f.Close()

	if _, err := io.Copy(f, resp.Body); err != nil {
		return "", fmt.Errorf("download: %w", err)
	}

	return localPath, nil
}

// WaitForCompletion polls /history until the prompt is done or timeout.
func (c *ComfyUIClient) WaitForCompletion(promptID string, timeout time.Duration) (*HistoryEntry, error) {
	deadline := time.Now().Add(timeout)
	poll := 500 * time.Millisecond

	for time.Now().Before(deadline) {
		entry, err := c.GetHistory(promptID)
		if err != nil {
			return nil, err
		}
		if entry != nil && entry.Status.Completed {
			return entry, nil
		}
		time.Sleep(poll)
		// Back off slightly
		if poll < 3*time.Second {
			poll = poll * 3 / 2
		}
	}
	return nil, fmt.Errorf("timeout waiting for prompt %s after %s", promptID, timeout)
}

// InjectPromptText modifies CLIPTextEncode nodes in a workflow to use the given prompt text.
// It scans the workflow for nodes of class_type "CLIPTextEncode" and sets their "text" input.
func InjectPromptText(workflow json.RawMessage, positive string, negative string) (json.RawMessage, error) {
	var nodes map[string]map[string]interface{}
	if err := json.Unmarshal(workflow, &nodes); err != nil {
		return nil, fmt.Errorf("unmarshal workflow: %w", err)
	}

	positiveSet := false
	for _, node := range nodes {
		classType, _ := node["class_type"].(string)
		if classType != "CLIPTextEncode" {
			continue
		}

		inputs, ok := node["inputs"].(map[string]interface{})
		if !ok {
			continue
		}

		// Heuristic: first CLIPTextEncode = positive, second = negative
		if !positiveSet {
			inputs["text"] = positive
			positiveSet = true
		} else if negative != "" {
			inputs["text"] = negative
			break
		}
	}

	return json.Marshal(nodes)
}

// Ping checks if the ComfyUI server is reachable.
func (c *ComfyUIClient) Ping() error {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(c.endpoint + "/object_info")
	if err != nil {
		return fmt.Errorf("cannot reach ComfyUI at %s: %w", c.endpoint, err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ComfyUI returned status %d", resp.StatusCode)
	}
	return nil
}
