package actionattrs

import (
	"bytes"
	"context"
	"io"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

var markdownRenderer = goldmark.New(goldmark.WithExtensions(extension.GFM))

type markdown struct{ source string }

// Markdown renders untrusted Markdown with bounded links and no raw HTML.
func Markdown(source string) markdown { return markdown{source: source} }

func (m markdown) Render(ctx context.Context, w io.Writer) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	rendered, err := renderMarkdown(m.source)
	if err != nil {
		return err
	}
	n, err := w.Write(rendered)
	if err == nil && n != len(rendered) {
		return io.ErrShortWrite
	}
	return err
}

// MarkdownHTML returns the same guarded markup used by Markdown for previews.
func MarkdownHTML(source string) (string, error) {
	rendered, err := renderMarkdown(source)
	return string(rendered), err
}

func renderMarkdown(source string) ([]byte, error) {
	var raw bytes.Buffer
	if err := markdownRenderer.Convert([]byte(source), &raw); err != nil {
		return nil, err
	}
	nodes, err := html.ParseFragment(&raw, &html.Node{Type: html.ElementNode, Data: "div", DataAtom: atom.Div})
	if err != nil {
		return nil, err
	}
	var output bytes.Buffer
	for _, node := range nodes {
		for element := range node.Descendants() {
			guardMarkdownLink(element)
		}
		guardMarkdownLink(node)
		if err := html.Render(&output, node); err != nil {
			return nil, err
		}
	}
	if err := scanRenderedHTML(output.Bytes()); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

func guardMarkdownLink(node *html.Node) {
	if node.Type != html.ElementNode || node.Data != "a" {
		return
	}
	for i := 0; i < len(node.Attr); {
		if node.Attr[i].Key == "href" && !validHref(node.Attr[i].Val) {
			node.Attr = append(node.Attr[:i], node.Attr[i+1:]...)
			continue
		}
		i++
	}
	node.Attr = append(node.Attr, html.Attribute{Key: "class", Val: "border rounded px-1"})
}
