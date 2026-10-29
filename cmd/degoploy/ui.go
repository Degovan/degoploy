package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"
	"golang.org/x/term"
)

// Plain ANSI colors follow the terminal's own theme; lipgloss drops them when
// output is not a terminal or NO_COLOR is set.
var (
	bold   = lipgloss.NewStyle().Bold(true)
	muted  = lipgloss.NewStyle().Foreground(lipgloss.BrightBlack)
	green  = lipgloss.NewStyle().Foreground(lipgloss.Green)
	yellow = lipgloss.NewStyle().Foreground(lipgloss.Yellow)
	red    = lipgloss.NewStyle().Foreground(lipgloss.Red)
	blue   = lipgloss.NewStyle().Foreground(lipgloss.Blue)
)

var formTheme = huh.ThemeFunc(huh.ThemeBase16)

// errCanceled ends a command the user backed out of (Esc or Ctrl+C in a form).
var errCanceled = errors.New("canceled")

// hintError is an error with follow-up advice printed under it.
type hintError struct {
	msg  string
	hint []string
}

func (e *hintError) Error() string { return e.msg }

func withHint(msg string, hint ...string) error { return &hintError{msg: msg, hint: hint} }

// interactive reports whether the CLI may ask questions.
func interactive() bool {
	return term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
}

// ask shows fields as one form.
func ask(fields ...huh.Field) error {
	err := huh.NewForm(huh.NewGroup(fields...)).WithTheme(formTheme).Run()
	if errors.Is(err, huh.ErrUserAborted) {
		return errCanceled
	}
	return err
}

func confirm(title, description string, yes bool) (bool, error) {
	err := ask(huh.NewConfirm().Title(title).Description(description).Value(&yes))
	return yes, err
}

// working runs fn behind a spinner titled with what it does.
func working(title string, fn func(context.Context) error) error {
	return workingWith(title, nil, fn)
}

// workingWith is working with a status, such as download progress, shown
// after the title. status is called on every frame, so it must be cheap.
func workingWith(title string, status func() string, fn func(context.Context) error) error {
	if !interactive() {
		lipgloss.Println(muted.Render(title + "…"))
		return fn(context.Background())
	}
	done := make(chan error, 1)
	go func() { done <- fn(context.Background()) }()
	frames := []rune("⣾⣽⣻⢿⡿⣟⣯⣷")
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for i := 0; ; i++ {
		line := blue.Render(string(frames[i%len(frames)])) + " " + title + "…"
		if status != nil {
			if s := status(); s != "" {
				line += "  " + muted.Render(s)
			}
		}
		lipgloss.Print("\r\033[K" + line)
		select {
		case err := <-done:
			lipgloss.Print("\r\033[K")
			return err
		case <-tick.C:
		}
	}
}

func success(format string, a ...any) {
	lipgloss.Println(green.Render("✓") + " " + fmt.Sprintf(format, a...))
}

func warning(format string, a ...any) {
	lipgloss.Fprintln(os.Stderr, yellow.Render("!")+" "+fmt.Sprintf(format, a...))
}

func printError(err error) {
	lipgloss.Fprintln(os.Stderr, red.Render("✗")+" "+err.Error())
	var h *hintError
	if errors.As(err, &h) {
		for _, line := range h.hint {
			lipgloss.Fprintln(os.Stderr, "  "+muted.Render(line))
		}
	}
}

// heading prints a title with a muted note after it.
func heading(title, note string) {
	if note != "" {
		title += "  " + muted.Render(note)
	}
	lipgloss.Println(bold.Render(title))
}

// fields prints aligned "key  value" lines.
func fields(rows [][2]string) {
	width := 0
	for _, r := range rows {
		width = max(width, len(r[0]))
	}
	for _, r := range rows {
		lipgloss.Println("  " + muted.Render(r[0]+strings.Repeat(" ", width-len(r[0]))) + "  " + r[1])
	}
}

// printTable prints rows under a header line, without other borders.
func printTable(headers []string, rows [][]string) {
	t := table.New().
		Headers(headers...).
		Rows(rows...).
		Border(lipgloss.NormalBorder()).
		BorderTop(false).BorderBottom(false).BorderLeft(false).BorderRight(false).
		BorderColumn(false).BorderRow(false).
		BorderStyle(muted).
		StyleFunc(func(row, _ int) lipgloss.Style {
			s := lipgloss.NewStyle().PaddingRight(3)
			if row == table.HeaderRow {
				return s.Inherit(muted)
			}
			return s
		})
	lipgloss.Println(t.String())
}
