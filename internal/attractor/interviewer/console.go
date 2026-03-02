package interviewer

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
)

// Console is an Interviewer that reads from stdin and writes to stdout.
type Console struct {
	In  io.Reader
	Out io.Writer
}

// NewConsole creates a Console using os.Stdin and os.Stdout.
func NewConsole() *Console {
	return &Console{In: os.Stdin, Out: os.Stdout}
}

func (c *Console) Ask(q Question) (Answer, error) {
	switch q.Type {
	case YesNo:
		return c.askYesNo(q)
	case MultipleChoice:
		return c.askMultipleChoice(q)
	case Confirmation:
		return c.askConfirmation(q)
	default:
		return c.askFreeform(q)
	}
}

func (c *Console) askYesNo(q Question) (Answer, error) {
	fmt.Fprintf(c.Out, "%s [y/n]: ", q.Prompt)
	line, err := c.readLine()
	if err != nil {
		return Answer{}, err
	}
	return Answer{Text: line}, nil
}

func (c *Console) askMultipleChoice(q Question) (Answer, error) {
	fmt.Fprintln(c.Out, q.Prompt)
	for i, opt := range q.Options {
		fmt.Fprintf(c.Out, "  %d) %s\n", i+1, opt)
	}
	fmt.Fprintf(c.Out, "Choice: ")
	line, err := c.readLine()
	if err != nil {
		return Answer{}, err
	}

	// Check accelerator keys first
	if idx := MatchAccelerator(line, q.Options); idx >= 0 {
		return Answer{Text: q.Options[idx], Selected: idx}, nil
	}

	// Try numeric selection
	var idx int
	if _, err := fmt.Sscanf(line, "%d", &idx); err == nil && idx >= 1 && idx <= len(q.Options) {
		return Answer{Text: q.Options[idx-1], Selected: idx - 1}, nil
	}

	return Answer{Text: line, Selected: -1}, nil
}

func (c *Console) askConfirmation(q Question) (Answer, error) {
	fmt.Fprintf(c.Out, "%s [Enter to confirm]: ", q.Prompt)
	line, err := c.readLine()
	if err != nil {
		return Answer{}, err
	}
	if line == "" {
		return Answer{Text: "yes"}, nil
	}
	return Answer{Text: line}, nil
}

func (c *Console) askFreeform(q Question) (Answer, error) {
	prompt := q.Prompt
	if q.Default != "" {
		prompt = fmt.Sprintf("%s [%s]: ", prompt, q.Default)
	} else {
		prompt += ": "
	}
	fmt.Fprint(c.Out, prompt)
	line, err := c.readLine()
	if err != nil {
		return Answer{}, err
	}
	if line == "" && q.Default != "" {
		line = q.Default
	}
	return Answer{Text: line}, nil
}

func (c *Console) readLine() (string, error) {
	scanner := bufio.NewScanner(c.In)
	if scanner.Scan() {
		return strings.TrimSpace(scanner.Text()), nil
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", io.EOF
}
