package atoms

import (
	"context"
	"strings"
	"testing"

	"github.com/a-h/templ"
)

func render(t *testing.T, c templ.Component) string {
	t.Helper()
	var b strings.Builder
	if err := c.Render(context.Background(), &b); err != nil {
		t.Fatalf("render: %v", err)
	}
	return b.String()
}

func TestCatalogNavigationHasVisibleBoundaries(t *testing.T) {
	feature := render(t, Feature(FeatureProps{Icon: PlainText("icon"), URL: "/feature"}))
	footer := render(t, FooterNav("Explore", []AnchorProps{{Label: "Documentation", Attrs: templ.Attributes{"href": "/docs"}}}))
	for _, tc := range []struct{ name, html, href string }{
		{"feature", feature, "/feature"},
		{"footer", footer, "/docs"},
	} {
		if !strings.Contains(tc.html, `href="`+tc.href+`"`) || !strings.Contains(tc.html, `class="btn btn-sm`) {
			t.Errorf("%s navigation needs a bounded link: %s", tc.name, tc.html)
		}
	}
}

func TestLoadingVariantDefault(t *testing.T) {
	got := render(t, Loading(LoadingProps{}))
	if !strings.Contains(got, "loading-spinner") {
		t.Errorf("default variant missing: %s", got)
	}
	got = render(t, Loading(LoadingProps{Variant: "dots"}))
	if !strings.Contains(got, "loading-dots") {
		t.Errorf("explicit variant missing: %s", got)
	}
}

func TestBarChartScalesToMax(t *testing.T) {
	got := render(t, BarChart(BarChartProps{Bars: []ChartBar{
		{Label: "a", Value: 50},
		{Label: "b", Value: 100},
	}}))
	if !strings.Contains(got, `height="100"`) {
		t.Errorf("tallest bar should be full height: %s", got)
	}
	if !strings.Contains(got, `aria-label="Bar chart, 2 series"`) {
		t.Errorf("aria-label missing: %s", got)
	}
}

func TestResizableDefaultsAndPane(t *testing.T) {
	got := render(t, Resizable(ResizableProps{ID: "r1", First: PlainText("a"), Second: PlainText("b")}))
	if !strings.Contains(got, `flex: 0 0 50%`) {
		t.Errorf("default 50%% split missing: %s", got)
	}
	if !strings.Contains(got, `min="20"`) || !strings.Contains(got, `max="80"`) {
		t.Errorf("default bounds missing: %s", got)
	}
	if !strings.Contains(got, `data-pane="r1-first"`) {
		t.Errorf("pane binding missing: %s", got)
	}
}

func TestPopoverAlignClass(t *testing.T) {
	got := render(t, Popover(PopoverPanelProps{Align: "end", Trigger: PlainText("open")}))
	if !strings.Contains(got, `dropdown-end`) {
		t.Errorf("align class missing: %s", got)
	}
	got = render(t, Popover(PopoverPanelProps{Trigger: PlainText("open")}))
	if !strings.Contains(got, `class="dropdown "`) {
		t.Errorf("no align class should be emitted: %s", got)
	}
}

func TestFilterResetLabelDefault(t *testing.T) {
	got := render(t, Filter(FilterProps{Name: "f", Options: []FilterOption{{Label: "All", Value: "all"}}}))
	if !strings.Contains(got, `value="×"`) {
		t.Errorf("default reset glyph missing: %s", got)
	}
	if !strings.Contains(got, `name="f"`) || !strings.Contains(got, `aria-label="All"`) {
		t.Errorf("radio name or aria-label missing: %s", got)
	}
}

func TestThemeControllerTypeDefault(t *testing.T) {
	got := render(t, ThemeController(ThemeControllerProps{Theme: "dark"}))
	if !strings.Contains(got, `type="checkbox"`) {
		t.Errorf("default type missing: %s", got)
	}
	got = render(t, ThemeController(ThemeControllerProps{Theme: "light", Type: "radio"}))
	if !strings.Contains(got, `type="radio"`) {
		t.Errorf("explicit type missing: %s", got)
	}
}

func TestJoinDirectionClasses(t *testing.T) {
	got := render(t, Join(JoinProps{Vertical: true}))
	if !strings.Contains(got, `join-vertical`) {
		t.Errorf("vertical class missing: %s", got)
	}
	got = render(t, Join(JoinProps{}))
	if !strings.Contains(got, `join-horizontal`) {
		t.Errorf("horizontal default missing: %s", got)
	}
}

func TestMaskShapeRequired(t *testing.T) {
	got := render(t, Mask(MaskProps{Shape: "squircle", Src: "a.png", Alt: "img"}))
	if !strings.Contains(got, `mask-squircle`) {
		t.Errorf("shape class missing: %s", got)
	}
	got = render(t, Mask(MaskProps{Shape: "circle", Half: "1", Src: "a.png"}))
	if !strings.Contains(got, `mask-half-1`) {
		t.Errorf("half class missing: %s", got)
	}
}

func TestProgressMaxDefault(t *testing.T) {
	got := render(t, Progress(ProgressProps{Value: 50}))
	if !strings.Contains(got, `max="100"`) {
		t.Errorf("default max missing: %s", got)
	}
	got = render(t, Progress(ProgressProps{Value: 1, Max: 10}))
	if !strings.Contains(got, `max="10"`) {
		t.Errorf("explicit max missing: %s", got)
	}
}
