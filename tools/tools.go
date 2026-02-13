package tools

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/openai/openai-go/v3"
)

//go:embed definitions/*.json
var toolsFS embed.FS

func LoadDefinitions() ([]openai.ChatCompletionToolUnionParam, error) {
	entries, err := toolsFS.ReadDir("definitions")
	if err != nil {
		return nil, fmt.Errorf("reading embedded tools: %w", err)
	}

	var tools []openai.ChatCompletionToolUnionParam
	for _, entry := range entries {
		data, err := toolsFS.ReadFile("definitions/" + entry.Name())
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", entry.Name(), err)
		}

		var tool openai.ChatCompletionToolUnionParam
		if err := json.Unmarshal(data, &tool); err != nil {
			return nil, fmt.Errorf("unmarshalling %s: %w", entry.Name(), err)
		}

		tools = append(tools, tool)
	}

	return tools, nil
}

func Execute(name, arguments string) string {
	switch name {
	case "Read":
		return executeRead(arguments)
	case "Write":
		return executeWrite(arguments)
	case "Bash":
		return executeBash(arguments)
	default:
		return fmt.Sprintf("unknown tool: %s", name)
	}
}

func executeRead(arguments string) string {
	var args struct {
		FilePath string `json:"file_path"`
	}

	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return fmt.Sprintf("error parsing arguments: %v", err)
	}

	data, err := os.ReadFile(args.FilePath)
	if err != nil {
		return fmt.Sprintf("error reading file: %v", err)
	}

	return string(data)
}

func executeWrite(arguments string) string {
	var args struct {
		FilePath string `json:"file_path"`
		Content  string `json:"content"`
	}

	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return fmt.Sprintf("error parsing arguments: %v", err)
	}

	file, err := os.Create(args.FilePath)
	if err != nil {
		return fmt.Sprintf("error creating or truncating file: %v", err)
	}
	defer file.Close()

	if _, err := file.WriteString(args.Content); err != nil {
		return fmt.Sprintf("error writing to file: %v", err)
	}

	return fmt.Sprintf("successfully written to: %s", args.FilePath)
}

func executeBash(arguments string) string {
	var args struct {
		Command string `json:"command"`
	}

	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return fmt.Sprintf("error parsing arguments: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	out, err := exec.CommandContext(ctx, "sh", "-c", args.Command).CombinedOutput()
	if err != nil {
		return fmt.Sprintf("%s\nerror: %v", out, err)
	}

	return string(out)
}
