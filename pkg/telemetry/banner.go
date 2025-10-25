package telemetry

import (
	"fmt"
	"io"
	"sync"
)

// BannerState is the live information shown by the portal client terminal.
type BannerState struct {
	PublicURL     string
	LocalTarget   string
	InspectorAddr string
	LiveRequests  int64
}

// Banner renders a refreshable tunnel status block to an io.Writer.
type Banner struct {
	mu     sync.Mutex
	writer io.Writer
}

// NewBanner creates a terminal banner renderer.
func NewBanner(writer io.Writer) *Banner { return &Banner{writer: writer} }

// Render redraws the current tunnel status.
func (b *Banner) Render(state BannerState) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.writer == nil {
		return fmt.Errorf("banner writer cannot be nil")
	}
	_, err := fmt.Fprintf(b.writer, "\033[2J\033[HPortal tunnel\n\n  Public URL   %s\n  Local target  %s\n  Inspector     %s\n  Live requests %d\n", state.PublicURL, state.LocalTarget, state.InspectorAddr, state.LiveRequests)
	return err
}

// Clear removes the rendered banner from a terminal.
func (b *Banner) Clear() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.writer == nil {
		return fmt.Errorf("banner writer cannot be nil")
	}
	_, err := io.WriteString(b.writer, "\033[2J\033[H")
	return err
}
