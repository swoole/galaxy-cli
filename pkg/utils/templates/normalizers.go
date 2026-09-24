package templates

import (
	"github.com/MakeNowJust/heredoc"
	"github.com/russross/blackfriday/v2"
	"strings"
)

const Indentation = `  `

func LongDesc(s string) string {
	if len(s) == 0 {
		return s
	}
	return normalizer{s}.heredoc().markdown().trim().string
}

// Examples normalizes a command's examples to follow the conventions.
func Examples(s string) string {
	if len(s) == 0 {
		return s
	}
	return normalizer{s}.trim().indent().string
}

type normalizer struct {
	string
}

func (s normalizer) markdown() normalizer {
	bytes := []byte(s.string)
	formatted := blackfriday.Run(bytes, blackfriday.WithExtensions(blackfriday.NoIntraEmphasis), blackfriday.WithRenderer(&ASCIIRenderer{Indentation: Indentation}))
	s.string = string(formatted)
	return s
}

func (s normalizer) heredoc() normalizer {
	s.string = heredoc.Doc(s.string)
	return s
}

func (s normalizer) trim() normalizer {
	s.string = strings.TrimSpace(s.string)
	return s
}

func (s normalizer) indent() normalizer {
	indentedLines := []string{}
	for _, line := range strings.Split(s.string, "\n") {
		trimmed := strings.TrimSpace(line)
		indented := Indentation + trimmed
		indentedLines = append(indentedLines, indented)
	}
	s.string = strings.Join(indentedLines, "\n")
	return s
}
