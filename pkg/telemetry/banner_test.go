package telemetry

import (
	"strings"
	"testing"
)

func TestBannerRender(t *testing.T) {
	var output strings.Builder
	banner := NewBanner(&output)
	if err := banner.Render(BannerState{
		PublicURL:     "https://demo.example.com",
		LocalTarget:   "127.0.0.1:3000",
		InspectorAddr: "127.0.0.1:4040",
		LiveRequests:  12,
	}); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"https://demo.example.com", "127.0.0.1:3000", "127.0.0.1:4040", "Live requests 12"} {
		if !strings.Contains(output.String(), expected) {
			t.Errorf("banner output does not contain %q: %q", expected, output.String())
		}
	}
}
