package tmux

import (
	"context"
	"reflect"
	"testing"

	"github.com/sschmerda/tmux-parator/internal/theme"
)

func TestOpenPopupLetsAppDrawBorder(t *testing.T) {
	runner := &recordingRunner{}

	if err := OpenPopup(context.Background(), runner, "tmux-parator", "90%", "90%", theme.Default()); err != nil {
		t.Fatalf("OpenPopup() unexpected error: %v", err)
	}

	want := []string{
		"tmux", "display-popup", "-E", "-B",
		"-s", "fg=#ffffff,bg=#2d2b55",
		"-w", "90%", "-h", "90%",
		"tmux-parator",
	}
	if !reflect.DeepEqual(runner.calls[0], want) {
		t.Fatalf("runner call = %#v, want %#v", runner.calls[0], want)
	}
}
