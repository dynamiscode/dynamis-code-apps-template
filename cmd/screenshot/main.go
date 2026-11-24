// Command screenshot renders registered synthetic components without an application server.
package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
	"golang.org/x/net/html"

	"example.com/dynamis-code/apps-template/internal/web"
	"example.com/dynamis-code/apps-template/internal/web/assets"
	"example.com/dynamis-code/apps-template/internal/web/components/atoms"
)

var fixtures = map[string]func() templ.Component{
	"alert": func() templ.Component { return atoms.AlertInfo("This is a synthetic example.") },
}

var pages = map[string]func() (templ.Component, error){
	"login": web.ScreenshotLogin,
}

var stylesheets = []string{"tokens.css", "components.css", "app.css", "clients.css"}

func document(ctx context.Context, name, theme string) ([]byte, error) {
	fixture, ok := fixtures[name]
	page, fullPage := pages[name]
	if !ok && !fullPage {
		return nil, fmt.Errorf("unknown fixture %q", name)
	}
	if theme != "light" && theme != "dark" && theme != "system" {
		return nil, fmt.Errorf("invalid theme %q (use light, dark, or system)", theme)
	}
	var component templ.Component
	if fullPage {
		var err error
		component, err = page()
		if err != nil {
			return nil, err
		}
	} else {
		component = fixture()
	}
	var body bytes.Buffer
	if err := component.Render(ctx, &body); err != nil {
		return nil, err
	}
	if fullPage {
		return offlinePage(body.Bytes(), theme)
	}
	if err := checkAssets(bytes.NewReader(body.Bytes())); err != nil {
		return nil, err
	}
	var out bytes.Buffer
	out.WriteString(`<!doctype html><html`)
	if theme != "system" {
		fmt.Fprintf(&out, ` data-theme="%s"`, theme)
	}
	out.WriteString(`><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><meta http-equiv="Content-Security-Policy" content="default-src 'none'; img-src data:; style-src 'unsafe-inline' data:; font-src data:"><title>Fixture</title><style>`)
	for _, name := range stylesheets {
		css, err := assets.Files.ReadFile(name)
		if err != nil {
			return nil, fmt.Errorf("missing stylesheet %s: %w", name, err)
		}
		if err := checkCSS(string(css)); err != nil {
			return nil, fmt.Errorf("stylesheet %s: %w", name, err)
		}
		out.Write(css)
		out.WriteByte('\n')
	}
	out.WriteString(`</style></head><body><main>`)
	out.Write(body.Bytes())
	out.WriteString(`</main></body></html>`)
	return out.Bytes(), nil
}

// offlinePage preserves the page and replaces its embedded asset URLs with data URLs.
func offlinePage(source []byte, theme string) ([]byte, error) {
	doc, err := html.Parse(bytes.NewReader(source))
	if err != nil {
		return nil, err
	}
	var walk func(*html.Node) error
	walk = func(n *html.Node) error {
		if n.Type == html.ElementNode {
			if n.Data == "head" {
				policy := &html.Node{Type: html.ElementNode, Data: "meta", Attr: []html.Attribute{
					{Key: "http-equiv", Val: "Content-Security-Policy"},
					{Key: "content", Val: "default-src 'none'; script-src data:; style-src data: 'unsafe-inline'; img-src data:; font-src data:; connect-src 'none'; form-action 'none'; base-uri 'none'"},
				}}
				n.InsertBefore(policy, n.FirstChild)
			}
			if n.Data == "html" {
				for i := range n.Attr {
					if n.Attr[i].Key == "data-theme" {
						n.Attr = append(n.Attr[:i], n.Attr[i+1:]...)
						break
					}
				}
				if theme != "system" {
					n.Attr = append(n.Attr, html.Attribute{Key: "data-theme", Val: theme})
				}
			}
			if n.Data == "style" || n.Data == "base" {
				return fmt.Errorf("unsupported <%s> in page", n.Data)
			}
			for i := range n.Attr {
				a := &n.Attr[i]
				if a.Key == "style" {
					if err := checkCSS(a.Val); err != nil {
						return err
					}
				}
				resource := a.Key == "src" || a.Key == "srcset" || a.Key == "poster" || (n.Data == "link" && a.Key == "href")
				if !resource || a.Val == "" || strings.HasPrefix(a.Val, "data:") || strings.HasPrefix(a.Val, "#") {
					continue
				}
				if a.Key == "srcset" || !strings.HasPrefix(a.Val, "/assets/") {
					return fmt.Errorf("missing offline asset: <%s %s=%q>", n.Data, a.Key, a.Val)
				}
				name := strings.TrimPrefix(a.Val, "/assets/")
				if path.Clean(name) != name || strings.HasPrefix(name, ".") {
					return fmt.Errorf("invalid asset path %q", a.Val)
				}
				content, err := assets.Files.ReadFile(name)
				if err != nil {
					return fmt.Errorf("missing offline asset %q: %w", a.Val, err)
				}
				kind := "application/octet-stream"
				switch path.Ext(name) {
				case ".css":
					kind = "text/css"
					if err := checkCSS(string(content)); err != nil {
						return fmt.Errorf("asset %s: %w", name, err)
					}
				case ".js":
					kind = "text/javascript"
				default:
					return fmt.Errorf("unsupported offline asset %q", a.Val)
				}
				a.Val = "data:" + kind + ";base64," + base64.StdEncoding.EncodeToString(content)
			}
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			if err := walk(child); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(doc); err != nil {
		return nil, err
	}
	var out bytes.Buffer
	if err := html.Render(&out, doc); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

var cssResource = regexp.MustCompile(`(?i)url\(\s*([^)]*)\)|@import\s+([^;]+)`)

func checkCSS(css string) error {
	for _, match := range cssResource.FindAllStringSubmatch(css, -1) {
		value := strings.Trim(strings.TrimSpace(match[1]), `"' `)
		if match[2] != "" || (!strings.HasPrefix(value, "data:") && !strings.HasPrefix(value, "#") && !strings.HasPrefix(value, "%23")) {
			return fmt.Errorf("missing offline asset: %s", match[0])
		}
	}
	return nil
}

// Fixtures must contain their resources inline; no paths can resolve offline.
func checkAssets(r io.Reader) error {
	z := html.NewTokenizer(r)
	for {
		switch z.Next() {
		case html.ErrorToken:
			if errors.Is(z.Err(), io.EOF) {
				return nil
			}
			return z.Err()
		case html.StartTagToken, html.SelfClosingTagToken:
			tag := z.Token()
			if tag.Data == "style" || tag.Data == "script" {
				return fmt.Errorf("fixture must not contain <%s>; embed assets through the renderer", tag.Data)
			}
			for _, a := range tag.Attr {
				if a.Key == "style" {
					if err := checkCSS(a.Val); err != nil {
						return err
					}
				}
				resource := a.Key == "src" || a.Key == "srcset" || a.Key == "poster" || (tag.Data == "link" && a.Key == "href")
				if resource && a.Val != "" && (a.Key == "srcset" || (!strings.HasPrefix(a.Val, "data:") && !strings.HasPrefix(a.Val, "#"))) {
					return fmt.Errorf("missing offline asset: <%s %s=%q>", tag.Data, a.Key, a.Val)
				}
			}
		}
	}
}

func screenshot(ctx context.Context, page []byte, width, height int64) ([]byte, error) {
	if width <= 0 || height <= 0 {
		return nil, errors.New("viewport dimensions must be positive")
	}
	alloc, cancel := chromedp.NewExecAllocator(ctx, append(chromedp.DefaultExecAllocatorOptions[:], chromedp.Flag("disable-background-networking", true))...)
	defer cancel()
	browser, cancel := chromedp.NewContext(alloc)
	defer cancel()
	browser, cancel = context.WithTimeout(browser, 30*time.Second)
	defer cancel()
	blocked := make(chan string, 1)
	if err := chromedp.Listen(browser, func(event any) {
		if failed, ok := event.(*network.EventLoadingFailed); ok && failed.BlockedReason != "" {
			select {
			case blocked <- failed.ErrorText:
			default:
			}
		}
	}); err != nil {
		return nil, fmt.Errorf("listen for blocked resources: %w", err)
	}
	if _, err := chromedp.Call(browser, network.Enable, network.EnableParams{}); err != nil {
		return nil, fmt.Errorf("start browser: %w", err)
	}
	_, err := chromedp.Call(browser, network.SetBlockedURLs, network.SetBlockedURLsParams{URLPatterns: []*network.BlockPattern{{URLPattern: "*://*/*", Block: true}}})
	if err != nil {
		return nil, fmt.Errorf("block network: %w", err)
	}
	if err := chromedp.Do(browser, chromedp.EmulateViewport(width, height), chromedp.Navigate("data:text/html;base64,"+base64.StdEncoding.EncodeToString(page)), chromedp.WaitReady("body")); err != nil {
		return nil, fmt.Errorf("render fixture: %w", err)
	}
	png, err := chromedp.Run(browser, chromedp.CaptureScreenshot())
	if err != nil {
		return nil, fmt.Errorf("capture screenshot: %w", err)
	}
	select {
	case reason := <-blocked:
		return nil, fmt.Errorf("blocked runtime resource: %s", reason)
	default:
	}
	return png, nil
}

func run() error {
	name := flag.String("fixture", "alert", "registered fixture name")
	theme := flag.String("theme", "light", "light, dark, or system")
	width := flag.Int64("width", 1280, "viewport width in CSS pixels")
	height := flag.Int64("height", 800, "viewport height in CSS pixels")
	htmlPath := flag.String("html", "fixture.html", "HTML output path")
	pngPath := flag.String("png", "fixture.png", "PNG output path")
	flag.Parse()
	if *htmlPath == "" || *pngPath == "" || *htmlPath == *pngPath {
		return errors.New("HTML and PNG output paths must be distinct and nonempty")
	}
	page, err := document(context.Background(), *name, *theme)
	if err != nil {
		return err
	}
	png, err := screenshot(context.Background(), page, *width, *height)
	if err != nil {
		return err
	}
	if err := os.WriteFile(*htmlPath, page, 0o644); err != nil {
		return err
	}
	return os.WriteFile(*pngPath, png, 0o644)
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
