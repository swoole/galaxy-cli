package cliui

import (
	"fmt"
	"io"
	"os"
	"strings"

	"charm.land/lipgloss/v2"
	liptable "charm.land/lipgloss/v2/table"
	"golang.org/x/term"
)

type Status string

const (
	StatusPass Status = "通过"
	StatusFail Status = "失败"
	StatusWarn Status = "警告"
	StatusInfo Status = "信息"
)

type Alignment int

const (
	AlignLeft Alignment = iota
	AlignCenter
	AlignRight
)

type Check struct {
	Status Status
	Name   string
	Detail string
}

type Pair struct {
	Label string
	Value string
}

type Column struct {
	Header   string
	WidthMax int
	Align    Alignment
}

type Renderer struct {
	output io.Writer
	color  bool
	width  int
}

func New(output io.Writer) Renderer {
	renderer := Renderer{output: output, width: 120}
	file, ok := output.(*os.File)
	if !ok {
		return renderer
	}
	renderer.color = os.Getenv("NO_COLOR") == "" &&
		!strings.EqualFold(os.Getenv("TERM"), "dumb") &&
		term.IsTerminal(int(file.Fd()))
	if width, _, err := term.GetSize(int(file.Fd())); err == nil && width >= 60 {
		renderer.width = width
	}
	return renderer
}

func (r Renderer) Title(title, subtitle string) {
	fmt.Fprintln(r.output, r.paint("\n◆ "+title, "\x1b[1;36m"))
	if strings.TrimSpace(subtitle) != "" {
		fmt.Fprintln(r.output, r.paint("  "+subtitle, "\x1b[2m"))
	}
}

func (r Renderer) Summary(pairs []Pair) {
	rows := make([][]string, 0, len(pairs))
	labelWidth := 10
	for _, pair := range pairs {
		rows = append(rows, []string{pair.Label, valueOrDash(pair.Value)})
		labelWidth = max(labelWidth, lipgloss.Width(pair.Label)+2)
	}
	width := min(r.width-2, 120)
	labelWidth = min(labelWidth, min(30, width/3))
	rendered := liptable.New().
		Border(lipgloss.RoundedBorder()).
		BorderRow(true).
		Width(width).
		Rows(rows...).
		StyleFunc(func(_ int, column int) lipgloss.Style {
			style := lipgloss.NewStyle().Padding(0, 1)
			if column == 0 {
				style = style.Width(labelWidth)
				if r.color {
					style = style.Bold(true).Foreground(lipgloss.BrightCyan)
				}
			}
			return style
		}).
		Render()
	fmt.Fprintln(r.output, rendered)
}

func (r Renderer) Checks(checks []Check) {
	rows := make([][]string, 0, len(checks))
	for _, check := range checks {
		rows = append(rows, []string{
			r.statusBadge(check.Status),
			check.Name,
			valueOrDash(check.Detail),
		})
	}
	r.Table([]Column{
		{Header: "状态", WidthMax: 10, Align: AlignCenter},
		{Header: "检查项", WidthMax: 24},
		{Header: "详情", WidthMax: max(40, min(100, r.width-42))},
	}, rows)
}

func (r Renderer) Table(columns []Column, rows [][]string) {
	headers := make([]string, len(columns))
	totalMax := len(columns) + 1
	for index, column := range columns {
		headers[index] = column.Header
		totalMax += column.WidthMax
	}
	tableWidth := min(r.width-2, totalMax)
	if tableWidth < 40 {
		tableWidth = 40
	}
	rendered := liptable.New().
		Border(lipgloss.RoundedBorder()).
		BorderRow(true).
		Width(tableWidth).
		Headers(headers...).
		Rows(rows...).
		StyleFunc(func(row, column int) lipgloss.Style {
			style := lipgloss.NewStyle().Padding(0, 1)
			if column < len(columns) {
				switch columns[column].Align {
				case AlignCenter:
					style = style.Align(lipgloss.Center)
				case AlignRight:
					style = style.Align(lipgloss.Right)
				default:
					style = style.Align(lipgloss.Left)
				}
			}
			if row == liptable.HeaderRow {
				style = style.Align(lipgloss.Center)
				if r.color {
					style = style.Bold(true).Foreground(lipgloss.BrightCyan)
				}
			}
			return style
		}).
		Render()
	fmt.Fprintln(r.output, rendered)
}

func (r Renderer) Decision(ok bool, message string) {
	if ok {
		fmt.Fprintln(r.output, r.paint("\n✓ "+message, "\x1b[1;32m"))
		return
	}
	fmt.Fprintln(r.output, r.paint("\n✗ "+message, "\x1b[1;31m"))
}

func (r Renderer) Hint(label, value string) {
	fmt.Fprintf(r.output, "%s %s\n", r.paint("→ "+label+":", "\x1b[1;36m"), value)
}

func (r Renderer) Empty(message string) {
	fmt.Fprintln(r.output, r.paint("○ "+message, "\x1b[33m"))
}

func (r Renderer) Success(message string) {
	fmt.Fprintln(r.output, r.paint("✓ "+message, "\x1b[32m"))
}

func (r Renderer) Warning(message string) {
	fmt.Fprintln(r.output, r.paint("▲ "+message, "\x1b[33m"))
}

func (r Renderer) Info(message string) {
	fmt.Fprintln(r.output, r.paint("● "+message, "\x1b[36m"))
}

func (r Renderer) State(value string) string {
	lower := strings.ToLower(strings.TrimSpace(value))
	switch {
	case strings.Contains(lower, "running"), strings.HasPrefix(lower, "up"), strings.Contains(lower, "active"):
		return r.paint("● "+value, "\x1b[32m")
	case strings.Contains(lower, "exit"), strings.Contains(lower, "dead"), strings.Contains(lower, "fail"):
		return r.paint("● "+value, "\x1b[31m")
	case lower == "":
		return "-"
	default:
		return r.paint("● "+value, "\x1b[33m")
	}
}

func (r Renderer) statusBadge(status Status) string {
	switch status {
	case StatusPass:
		return r.paint("● 通过", "\x1b[32m")
	case StatusFail:
		return r.paint("● 失败", "\x1b[31m")
	case StatusWarn:
		return r.paint("▲ 警告", "\x1b[33m")
	default:
		return r.paint("● 信息", "\x1b[36m")
	}
}

func (r Renderer) paint(value, escape string) string {
	if !r.color {
		return value
	}
	return escape + value + "\x1b[0m"
}

func valueOrDash(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}

func min(left, right int) int {
	if left < right {
		return left
	}
	return right
}

func max(left, right int) int {
	if left > right {
		return left
	}
	return right
}
