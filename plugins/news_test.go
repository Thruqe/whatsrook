package commands

import (
	"testing"
)

func TestNewsCommandRegistration(t *testing.T) {
	cmd, ok := Get("news")
	if !ok {
		t.Fatal("expected 'news' command to be registered")
	}

	if cmd.Category != "info" {
		t.Errorf("expected category 'info', got %q", cmd.Category)
	}

	if !cmd.IsPublic {
		t.Error("expected news command to be public")
	}

	foundAlias := false
	for _, a := range cmd.Aliases {
		if a == "apnews" {
			foundAlias = true
			break
		}
	}
	if !foundAlias {
		t.Error("expected alias 'apnews' to be registered")
	}
}

func TestParseAPNewsHTML(t *testing.T) {
	sampleHTML := `<div class="PagePromo" data-gtm-region="Top News">
    <div class="PagePromo-media">
        <a class="Link" href="https://apnews.com/article/nigeria-beaches-inequality-economy-lagos-3436777e99a777e11840597d96ef33c4">
            <picture>
                <img class="Image" src="https://dims.apnews.com/dims4/default/9e43f2c/test.jpg" />
            </picture>
        </a>
    </div>
    <div class="PagePromo-content">
        <h3 class="PagePromo-title">
            <a class="Link" href="https://apnews.com/article/nigeria-beaches-inequality-economy-lagos-3436777e99a777e11840597d96ef33c4"><span class="PagePromoContentIcons-text">A beach break in Nigeria’s Lagos is becoming too expensive for many</span></a>
        </h3>
        <div class="PagePromo-description">
            <span class="PagePromoContentIcons-text">The beaches in Nigeria’s megacity of Lagos are rare public spaces to escape the crowds.</span>
        </div>
    </div>
</div></div></div>`

	articles := parseAPNewsHTML(sampleHTML)
	if len(articles) != 1 {
		t.Fatalf("expected 1 article, got %d", len(articles))
	}

	art := articles[0]
	if art.Title != "A beach break in Nigeria’s Lagos is becoming too expensive for many" {
		t.Errorf("unexpected title: %q", art.Title)
	}
	if art.URL != "https://apnews.com/article/nigeria-beaches-inequality-economy-lagos-3436777e99a777e11840597d96ef33c4" {
		t.Errorf("unexpected url: %q", art.URL)
	}
	if art.ImageURL != "https://dims.apnews.com/dims4/default/9e43f2c/test.jpg" {
		t.Errorf("unexpected image url: %q", art.ImageURL)
	}
}
