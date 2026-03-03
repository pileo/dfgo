package profile

// Anthropic profile uses edit_file and all standard tools.
type Anthropic struct{}

func (Anthropic) Name() string { return "anthropic" }
func (Anthropic) CoreTools() []string {
	return []string{"read_file", "write_file", "edit_file", "shell", "grep", "glob"}
}
func (Anthropic) EditTool() string        { return "edit_file" }
func (Anthropic) ContextWindowSize() int  { return 200000 }
func (Anthropic) SystemPrompt() string {
	return `You are an autonomous coding agent. You have access to tools for reading, writing, and editing files, running shell commands, and searching code. Use these tools to complete the user's task.

Key guidelines:
- Read files before editing them
- Use edit_file for precise string replacements
- Run tests after making changes
- If a tool call fails, analyze the error and adjust your approach
- Explain your reasoning before taking actions`
}
