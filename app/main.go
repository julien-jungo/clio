package main

import (
	"log"
	"os"

	"github.com/julien-jungo/clio/tools"
	"github.com/julien-jungo/clio/ui"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

func main() {
	baseUrl := os.Getenv("OPENROUTER_BASE_URL")
	if baseUrl == "" {
		baseUrl = "https://openrouter.ai/api/v1"
	}

	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		log.Fatal("OPENROUTER_API_KEY not found")
	}

	model := os.Getenv("CLIO_MODEL")
	if model == "" {
		model = "anthropic/claude-haiku-4.5"
	}

	toolDefs, err := tools.LoadDefinitions()
	if err != nil {
		log.Fatal(err)
	}

	client := openai.NewClient(option.WithAPIKey(apiKey), option.WithBaseURL(baseUrl))

	p := tea.NewProgram(ui.NewModel(client, toolDefs, model), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		log.Fatal(err)
	}
}
