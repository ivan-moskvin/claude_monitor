package divoom

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/ivan-moskvin/claude_monitor/i18n"
)

func TestTextWidth(t *testing.T) {
	cases := []struct {
		text  string
		scale int
		want  int
	}{
		{"", 1, 0},
		{"", 2, 0},
		{"5", 1, glyphWidth},
		// Two glyphs and one column of spacing between them.
		{"5H", 1, 2*glyphWidth + 1},
		{"5H", 2, 2*(2*glyphWidth+2) - 2},
		{"СБРОС", 2, 5*(2*glyphWidth+2) - 2},
	}

	for _, c := range cases {
		if got := textWidth(c.text, c.scale); got != c.want {
			t.Errorf("textWidth(%q, %d) = %d; want %d", c.text, c.scale, got, c.want)
		}
	}
}

// panelLabels lists every string the panel can draw, in a given language.
// Sources: render, drawBar labels and the two error lines.
func panelLabels(lang i18n.Lang) []string {
	return []string{
		"CLAUDE", "5H", "7D",
		i18n.In(lang, "NO"),
		i18n.In(lang, "DATA"),
		i18n.In(lang, "RESET"),
		fmt.Sprintf(i18n.In(lang, "%dH"), 12),
		fmt.Sprintf(i18n.In(lang, "%dM"), 59),
		countdownLabel(4*3600 + 59*60),
		usageWindow{present: true, used: 100}.percentLabel(),
		usageWindow{}.percentLabel(),
	}
}

// Every character of every label needs a glyph: an unknown rune is drawn as "?"
// and the screen shows nonsense — which is only visible on the device itself,
// and only to whoever runs the utility in that language.
func TestPanelLabelsHaveGlyphs(t *testing.T) {
	for _, lang := range i18n.Langs() {
		for _, label := range panelLabels(lang) {
			for _, r := range label {
				if _, ok := glyphs[r]; !ok {
					t.Errorf("%s: label %q uses %q, which has no glyph in font.go", lang, label, r)
				}
			}
		}
	}
}

// A label has to fit its bar as well: the font has no kerning, and a Russian
// word is not always as short as the English one it replaces.
func TestPanelLabelsFit(t *testing.T) {
	for _, lang := range i18n.Langs() {
		// The labels inside the bars are drawn at scale 2 (see drawBar).
		const labelScale = 2
		inBar := []string{
			i18n.In(lang, "RESET"),
			fmt.Sprintf(i18n.In(lang, "%dH"), 12),
			countdownLabel(4*3600 + 59*60),
			usageWindow{present: true, used: 100}.percentLabel(),
		}
		for _, label := range inBar {
			if width := textWidth(label, labelScale); width > barWidth {
				t.Errorf("%s: label %q is %d pixels wide, the bar is %d", lang, label, width, barWidth)
			}
		}

		// The "no data" lines are centered on the panel itself.
		if width := textWidth(i18n.In(lang, "NO"), 3); width > panelSize {
			t.Errorf("%s: %q does not fit the panel at scale 3", lang, i18n.In(lang, "NO"))
		}
		if width := textWidth(i18n.In(lang, "DATA"), 2); width > panelSize {
			t.Errorf("%s: %q does not fit the panel at scale 2", lang, i18n.In(lang, "DATA"))
		}
	}
}

func TestDrawTextFallsBackToQuestionMark(t *testing.T) {
	// "Ж" has no glyph — nothing in the interface needs it.
	if _, ok := glyphs['Ж']; ok {
		t.Skip("Ж has a glyph now; the test needs another character without one")
	}

	unknown, fallback := newPanel(), newPanel()
	unknown.drawText("Ж", 10, 10, 2, idxWhite)
	fallback.drawText("?", 10, 10, 2, idxWhite)

	if !bytes.Equal(unknown.img.Pix, fallback.img.Pix) {
		t.Error("a rune without a glyph is drawn as something other than ?")
	}
}

func TestDrawTextSplitPaintsEitherSideOfTheSplit(t *testing.T) {
	p := newPanel()
	// "8" fills its whole 5×7 box on the top row, so both colors are certain to
	// appear whatever the glyph looks like.
	p.drawTextSplit("88", 0, 0, 1, glyphWidth, idxWhite, idxGrey)

	var left, right bool
	for y := 0; y < glyphHeight; y++ {
		for x := 0; x < 2*glyphWidth+1; x++ {
			switch p.img.ColorIndexAt(x, y) {
			case idxWhite:
				left = true
				if x >= glyphWidth {
					t.Fatalf("the left color leaked past the split at x=%d", x)
				}
			case idxGrey:
				right = true
				if x < glyphWidth {
					t.Fatalf("the right color leaked before the split at x=%d", x)
				}
			}
		}
	}
	if !left || !right {
		t.Errorf("drawTextSplit painted only one side: left=%t right=%t", left, right)
	}
}

func TestDrawTextCenteredIsSymmetric(t *testing.T) {
	p := newPanel()
	p.drawTextCentered("88", 0, 2, idxWhite)

	minX, maxX := panelSize, -1
	for y := 0; y < panelSize; y++ {
		for x := 0; x < panelSize; x++ {
			if p.img.ColorIndexAt(x, y) == idxWhite {
				minX, maxX = min(minX, x), max(maxX, x)
			}
		}
	}
	if maxX < 0 {
		t.Fatal("nothing was drawn")
	}
	// The margins may differ by one pixel on an odd width, no more.
	if margin := minX - (panelSize - 1 - maxX); margin < -1 || margin > 1 {
		t.Errorf("the text is off center: %d pixels on the left, %d on the right", minX, panelSize-1-maxX)
	}
}
