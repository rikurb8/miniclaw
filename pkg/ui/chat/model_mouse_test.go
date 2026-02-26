package chat

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestHandleViewportMouseWheelUpDisablesFollowLog(t *testing.T) {
	t.Parallel()

	m := newModel(context.Background(), nil, modeInteractive, "", RuntimeInfo{})
	m.viewport.Width = 40
	m.viewport.Height = 5
	m.viewport.SetContent(strings.Repeat("line\n", 40))
	m.viewport.GotoBottom()
	m.followLog = true

	previousOffset := m.viewport.YOffset
	handled := m.handleViewportMouse(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonWheelUp})
	if !handled {
		t.Fatal("expected wheel-up mouse event to be handled")
	}
	if m.followLog {
		t.Fatal("expected followLog to be disabled after wheel-up scroll")
	}
	if m.viewport.YOffset >= previousOffset {
		t.Fatalf("expected YOffset to decrease after wheel-up scroll, got %d want < %d", m.viewport.YOffset, previousOffset)
	}
}

func TestHandleViewportMouseWheelDownAtBottomEnablesFollowLog(t *testing.T) {
	t.Parallel()

	m := newModel(context.Background(), nil, modeInteractive, "", RuntimeInfo{})
	m.viewport.Width = 40
	m.viewport.Height = 5
	m.viewport.SetContent(strings.Repeat("line\n", 40))
	m.viewport.GotoBottom()

	maxOffset := m.viewport.TotalLineCount() - m.viewport.Height
	m.viewport.SetYOffset(max(0, maxOffset-1))
	m.followLog = false

	handled := m.handleViewportMouse(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonWheelDown})
	if !handled {
		t.Fatal("expected wheel-down mouse event to be handled")
	}
	if !m.viewport.AtBottom() {
		t.Fatalf("expected viewport to reach bottom, got YOffset=%d", m.viewport.YOffset)
	}
	if !m.followLog {
		t.Fatal("expected followLog to re-enable when wheel-down reaches bottom")
	}
}

func TestHandleViewportMouseIgnoresNonWheelEvents(t *testing.T) {
	t.Parallel()

	m := newModel(context.Background(), nil, modeInteractive, "", RuntimeInfo{})
	handled := m.handleViewportMouse(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	if handled {
		t.Fatal("expected non-wheel mouse event to be ignored")
	}
}
