package ui

import (
	"context"
	"fmt"
	"strings"

	"github.com/julien-jungo/clio/tools"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/openai/openai-go/v3"
)

const (
	grey235 = lipgloss.Color("235")
	grey240 = lipgloss.Color("240")
	red     = lipgloss.Color("1")
)

var (
	userStyle      = lipgloss.NewStyle().Padding(0, 1).Background(grey235)
	assistantStyle = lipgloss.NewStyle().Padding(0, 1)
	toolCallStyle  = lipgloss.NewStyle().Padding(0, 1).Foreground(grey240)
	errorStyle     = lipgloss.NewStyle().Padding(0, 1).Foreground(red)
	welcomeStyle   = lipgloss.NewStyle().Padding(0, 1).Foreground(grey240)
	spinnerStyle   = lipgloss.NewStyle().Padding(0, 1)
	inputStyle     = lipgloss.NewStyle().Padding(0, 1)
)

type apiResponseMsg struct {
	choice openai.ChatCompletionChoice
}

type apiErrorMsg struct {
	err error
}

type toolResultsMsg struct {
	results []openai.ChatCompletionMessageParamUnion
}

type Model struct {
	viewport    viewport.Model
	textarea    textarea.Model
	spinner     spinner.Model
	messages    []string
	apiMessages []openai.ChatCompletionMessageParamUnion
	tools       []openai.ChatCompletionToolUnionParam
	client      openai.Client
	model       string
	waiting     bool
}

func NewModel(client openai.Client, toolDefs []openai.ChatCompletionToolUnionParam, model string) Model {
	ta := textarea.New()
	ta.Prompt = "▶ "
	ta.Placeholder = "Send a message..."
	ta.Focus()
	ta.CharLimit = 0
	ta.SetHeight(1)
	ta.MaxHeight = 0
	ta.ShowLineNumbers = false
	ta.KeyMap.InsertNewline.SetEnabled(false)
	ta.FocusedStyle.CursorLine = lipgloss.NewStyle()

	vp := viewport.New(80, 20)

	sp := spinner.New()
	sp.Spinner = spinner.Dot

	return Model{
		messages: []string{welcomeStyle.Render("Welcome! Type a message and press Enter to chat.")},
		apiMessages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage("You are clio, a coding assistant running in the user's terminal. " +
				"Use your tools to explore the codebase and make changes. Be concise in your responses."),
		},
		viewport: vp,
		textarea: ta,
		spinner:  sp,
		tools:    toolDefs,
		client:   client,
		model:    model,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(textarea.Blink, m.spinner.Tick)
}

func (m Model) sendToAPI() tea.Cmd {
	messages := make([]openai.ChatCompletionMessageParamUnion, len(m.apiMessages))
	copy(messages, m.apiMessages)
	toolDefs := m.tools
	client := m.client
	model := m.model

	return func() tea.Msg {
		resp, err := client.Chat.Completions.New(context.Background(),
			openai.ChatCompletionNewParams{
				Model:    model,
				Messages: messages,
				Tools:    toolDefs,
			},
		)
		if err != nil {
			return apiErrorMsg{err: err}
		}
		if len(resp.Choices) == 0 {
			return apiErrorMsg{err: fmt.Errorf("no choices in response")}
		}

		return apiResponseMsg{choice: resp.Choices[0]}
	}
}

func (m *Model) refreshViewport() {
	content := strings.Join(m.messages, "\n\n")

	if m.waiting {
		content += "\n\n" + spinnerStyle.Render(m.spinner.View()+" Thinking...")
	}

	wrapped := lipgloss.NewStyle().Width(m.viewport.Width).Render(content)
	m.viewport.SetContent(wrapped)
	m.viewport.GotoBottom()
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.handleWindowSize(msg)

	case tea.KeyMsg:
		if cmd := m.handleKeyPress(msg); cmd != nil {
			return m, cmd
		}

	case apiResponseMsg:
		if cmd := m.handleAPIResponse(msg); cmd != nil {
			return m, cmd
		}

	case toolResultsMsg:
		if cmd := m.handleToolResults(msg); cmd != nil {
			return m, cmd
		}

	case apiErrorMsg:
		m.handleAPIError(msg)

	case spinner.TickMsg:
		if cmd := m.handleSpinnerTick(msg); cmd != nil {
			cmds = append(cmds, cmd)
		}
	}

	var taCmd, vpCmd tea.Cmd
	m.textarea, taCmd = m.textarea.Update(msg)
	m.viewport, vpCmd = m.viewport.Update(msg)
	cmds = append(cmds, taCmd, vpCmd)

	return m, tea.Batch(cmds...)
}

func (m *Model) handleWindowSize(msg tea.WindowSizeMsg) {
	m.textarea.SetWidth(msg.Width)
	m.viewport.Width = msg.Width
	m.viewport.Height = msg.Height - m.textarea.Height() - 2
	m.refreshViewport()
}

func (m *Model) handleKeyPress(msg tea.KeyMsg) tea.Cmd {
	switch msg.Type {

	case tea.KeyCtrlC:
		return tea.Quit

	case tea.KeyEnter:
		if m.waiting {
			return nil
		}

		userMsg := strings.TrimSpace(m.textarea.Value())
		if userMsg == "" {
			return nil
		}

		m.messages = append(m.messages, userStyle.Width(m.viewport.Width).Render("▶ "+userMsg))
		m.apiMessages = append(m.apiMessages, openai.UserMessage(userMsg))

		m.textarea.Reset()
		m.waiting = true
		m.refreshViewport()

		return tea.Batch(m.sendToAPI(), m.spinner.Tick)
	}

	return nil
}

func (m *Model) handleAPIResponse(msg apiResponseMsg) tea.Cmd {
	choice := msg.choice

	toolCallParams := make([]openai.ChatCompletionMessageToolCallUnionParam, len(choice.Message.ToolCalls))
	for i, tc := range choice.Message.ToolCalls {
		toolCallParams[i] = tc.ToParam()
	}

	m.apiMessages = append(m.apiMessages, openai.ChatCompletionMessageParamUnion{
		OfAssistant: &openai.ChatCompletionAssistantMessageParam{
			Content: openai.ChatCompletionAssistantMessageParamContentUnion{
				OfString: openai.String(choice.Message.Content),
			},
			ToolCalls: toolCallParams,
		},
	})

	if len(choice.Message.ToolCalls) > 0 {
		for _, toolCall := range choice.Message.ToolCalls {
			m.messages = append(m.messages, toolCallStyle.Render(
				fmt.Sprintf("⚡ %s(%s)", toolCall.Function.Name, toolCall.Function.Arguments),
			))
		}

		m.refreshViewport()

		return m.executeTools(choice.Message.ToolCalls)
	}

	m.messages = append(m.messages, assistantStyle.Width(m.viewport.Width).Render(choice.Message.Content))
	m.waiting = false
	m.refreshViewport()

	return nil
}

func (m *Model) executeTools(toolCalls []openai.ChatCompletionMessageToolCallUnion) tea.Cmd {
	return func() tea.Msg {
		var results []openai.ChatCompletionMessageParamUnion
		for _, toolCall := range toolCalls {
			result := tools.Execute(toolCall.Function.Name, toolCall.Function.Arguments)
			results = append(results, openai.ToolMessage(result, toolCall.ID))
		}

		return toolResultsMsg{results: results}
	}
}

func (m *Model) handleToolResults(msg toolResultsMsg) tea.Cmd {
	m.apiMessages = append(m.apiMessages, msg.results...)
	return m.sendToAPI()
}

func (m *Model) handleAPIError(msg apiErrorMsg) {
	m.messages = append(m.messages, errorStyle.Width(m.viewport.Width).Render("✖ "+msg.err.Error()))
	m.waiting = false
	m.refreshViewport()
}

func (m *Model) handleSpinnerTick(msg spinner.TickMsg) tea.Cmd {
	if m.waiting {
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		m.refreshViewport()
		return cmd
	}

	return nil
}

func (m Model) View() string {
	return fmt.Sprintf("%s\n\n%s", m.viewport.View(), inputStyle.Render(m.textarea.View()))
}
