package profile

// OpenAI profile uses apply_patch instead of edit_file.
type OpenAI struct{}

func (OpenAI) Name() string { return "openai" }
func (OpenAI) CoreTools() []string {
	return []string{"read_file", "write_file", "apply_patch", "shell", "grep", "glob"}
}
func (OpenAI) EditTool() string        { return "apply_patch" }
func (OpenAI) ContextWindowSize() int  { return 128000 }
func (OpenAI) SystemPrompt() string {
	return `You are an autonomous coding agent. You have access to tools for reading, writing, and editing files, running shell commands, and searching code. Use these tools to complete the user's task.

Key guidelines:
- Read files before editing them
- Use apply_patch to make changes with unified diffs
- Run tests after making changes
- If a tool call fails, analyze the error and adjust your approach
- Explain your reasoning before taking actions`
}
