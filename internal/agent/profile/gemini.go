package profile

// GeminiProfile uses edit_file and adds web search capability.
type Gemini struct{}

func (Gemini) Name() string { return "gemini" }
func (Gemini) CoreTools() []string {
	return []string{"read_file", "write_file", "edit_file", "shell", "grep", "glob"}
}
func (Gemini) EditTool() string        { return "edit_file" }
func (Gemini) ContextWindowSize() int  { return 1000000 }
func (Gemini) SystemPrompt() string {
	return `You are an autonomous coding agent. You have access to tools for reading, writing, and editing files, running shell commands, and searching code. Use these tools to complete the user's task.

Key guidelines:
- Read files before editing them
- Use edit_file for precise string replacements
- Run tests after making changes
- If a tool call fails, analyze the error and adjust your approach
- Explain your reasoning before taking actions`
}
