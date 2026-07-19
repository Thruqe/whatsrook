package ember

import "testing"

func TestBestMedia(t *testing.T) {
	d := Data{
		Medias: []Media{
			{URL: "https://example.com/audio.mp3", Type: "audio"},
			{URL: "https://example.com/video.mp4", Type: "video"},
		},
	}
	m, ok := d.BestMedia()
	if !ok {
		t.Fatal("Expected best media to be found")
	}
	if m.Type != "video" {
		t.Errorf("Expected best media to be video, got %s", m.Type)
	}
}

func TestCaption(t *testing.T) {
	d := Data{
		Title:  "Hello World",
		Author: "Jane Doe",
	}
	cap := d.Caption()
	expected := "Hello World\n— Jane Doe"
	if cap != expected {
		t.Errorf("Expected caption %q, got %q", expected, cap)
	}
}
