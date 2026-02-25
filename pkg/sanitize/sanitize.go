package sanitize

import (
	"bytes"
	"strings"
	"unicode/utf8"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	gmtext "github.com/yuin/goldmark/text"
	"golang.org/x/net/html"
)

// CommentBodyReport describes what was detected/changed while sanitizing a comment body.
type CommentBodyReport struct {
	Changed bool

	// Input normalization / cleanup
	InvalidUTF8Fixed         bool
	DroppedFrontmatterBreaks int // number of standalone "---" lines removed
	RemovedNULBytes          bool

	// HTML detected (tags/comments/doctypes are removed from output)
	HTMLTagTokens     int
	HTMLCommentTokens int
	HTMLDoctypeTokens int

	// Markdown constructs detected and degraded/removed
	MarkdownLinks  int
	MarkdownImages int
}

// SanitizeCommentBodyWithReport sanitizes comment text and returns a report
// describing what was detected/changed.
//
// Allowed formatting: bold, italic, inline code, blockquotes.
// Disallowed: links, images, raw HTML.
func SanitizeCommentBodyWithReport(input string) (string, CommentBodyReport) {
	var rep CommentBodyReport

	original := input

	// Normalize newlines early.
	input = strings.ReplaceAll(input, "\r\n", "\n")
	input = strings.ReplaceAll(input, "\r", "\n")

	// Drop standalone YAML frontmatter breaker lines.
	lines := strings.Split(input, "\n")
	filteredLines := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) == "---" {
			rep.DroppedFrontmatterBreaks++
			continue
		}
		filteredLines = append(filteredLines, line)
	}
	input = strings.Join(filteredLines, "\n")

	// Ensure valid UTF-8 (remove invalid sequences).
	if !utf8.ValidString(input) {
		rep.InvalidUTF8Fixed = true
		input = strings.ToValidUTF8(input, "")
	}

	// Strip NUL bytes (defensive).
	if strings.IndexByte(input, 0x00) >= 0 {
		rep.RemovedNULBytes = true
		input = strings.ReplaceAll(input, "\x00", "")
	}

	// Step 1: remove HTML markup by extracting only text tokens + collect HTML token stats.
	plain, htmlTags, htmlComments, htmlDoctypes := stripHTMLToTextWithStats(input)
	rep.HTMLTagTokens = htmlTags
	rep.HTMLCommentTokens = htmlComments
	rep.HTMLDoctypeTokens = htmlDoctypes

	// Step 2: parse Markdown into AST.
	md := goldmark.New()
	reader := gmtext.NewReader([]byte(plain))
	doc := md.Parser().Parse(reader)

	// Step 3: re-render allowlisted nodes back to "safe markdown", collecting AST stats.
	out := renderAllowedMarkdownWithReport(doc, []byte(plain), &rep)

	// Normalize trailing newline (exactly one).
	out = strings.ReplaceAll(out, "\r\n", "\n")
	out = strings.TrimRight(out, "\n") + "\n"

	// Compute changed flag against original (also account for normalization).
	if out != original {
		rep.Changed = true
	}

	return out, rep
}

// Keep existing SanitizeCommentBody API intact.
func SanitizeCommentBody(input string) string {
	out, _ := SanitizeCommentBodyWithReport(input)
	return out
}

// stripHTMLToTextWithStats performs its package-specific operation.
func stripHTMLToTextWithStats(s string) (text string, tagTokens int, commentTokens int, doctypeTokens int) {
	var b strings.Builder
	b.Grow(len(s))

	z := html.NewTokenizer(strings.NewReader(s))
	for {
		tt := z.Next()
		switch tt {
		case html.ErrorToken:
			return b.String(), tagTokens, commentTokens, doctypeTokens

		case html.TextToken:
			b.Write(z.Text())

		case html.StartTagToken, html.EndTagToken, html.SelfClosingTagToken:
			tagTokens++

		case html.CommentToken:
			commentTokens++

		case html.DoctypeToken:
			doctypeTokens++

		default:
			// Ignore anything else.
		}
	}
}

// renderAllowedMarkdownWithReport performs its package-specific operation.
func renderAllowedMarkdownWithReport(doc ast.Node, source []byte, rep *CommentBodyReport) string {
	var b strings.Builder

	for n := doc.FirstChild(); n != nil; n = n.NextSibling() {
		b.WriteString(renderNodeWithReport(n, source, rep))
		if n.NextSibling() != nil && isBlockNode(n) {
			b.WriteString("\n")
		}
	}

	return b.String()
}

// renderNodeWithReport performs its package-specific operation.
func renderNodeWithReport(n ast.Node, source []byte, rep *CommentBodyReport) string {
	switch x := n.(type) {
	case *ast.Paragraph:
		s := renderInlineChildrenWithReport(n, source, rep)
		s = strings.TrimRight(s, " \t")
		if s == "" {
			return ""
		}
		return s + "\n"

	case *ast.Text:
		seg := x.Segment
		txt := seg.Value(source)
		out := escapeText(string(txt))
		if x.HardLineBreak() || x.SoftLineBreak() {
			out += "\n"
		}
		return out

	case *ast.Emphasis:
		content := renderInlineChildrenWithReport(n, source, rep)
		if content == "" {
			return ""
		}
		// goldmark: Level 1 = italic, Level 2 = bold
		if x.Level == 2 {
			return "**" + content + "**"
		}
		return "*" + content + "*"

	case *ast.CodeSpan:
		seg := x.Text(source)
		code := string(seg)
		code = strings.ReplaceAll(code, "\r\n", "\n")
		code = strings.ReplaceAll(code, "\r", "\n")

		delim := "`"
		if strings.Contains(code, "`") {
			delim = "``"
			if strings.Contains(code, "``") {
				delim = "```"
			}
		}
		return delim + code + delim

	case *ast.Blockquote:
		raw := renderBlockChildrenWithReport(n, source, rep)
		raw = strings.TrimRight(raw, "\n")
		if raw == "" {
			return ""
		}
		lines := strings.Split(raw, "\n")
		for i := range lines {
			if strings.TrimSpace(lines[i]) == "" {
				lines[i] = ">"
			} else {
				lines[i] = "> " + lines[i]
			}
		}
		return strings.Join(lines, "\n") + "\n"

	// Disallowed / degraded nodes:
	case *ast.Link:
		if rep != nil {
			rep.MarkdownLinks++
		}
		return renderInlineChildrenWithReport(n, source, rep)

	case *ast.Image:
		if rep != nil {
			rep.MarkdownImages++
		}
		return renderInlineChildrenWithReport(n, source, rep)

	default:
		if n.HasChildren() {
			if isBlockNode(n) {
				return renderBlockChildrenWithReport(n, source, rep)
			}
			return renderInlineChildrenWithReport(n, source, rep)
		}
		return ""
	}
}

// renderInlineChildrenWithReport performs its package-specific operation.
func renderInlineChildrenWithReport(n ast.Node, source []byte, rep *CommentBodyReport) string {
	var b strings.Builder
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		b.WriteString(renderNodeWithReport(c, source, rep))
	}
	return b.String()
}

// renderBlockChildrenWithReport performs its package-specific operation.
func renderBlockChildrenWithReport(n ast.Node, source []byte, rep *CommentBodyReport) string {
	var b strings.Builder
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		b.WriteString(renderNodeWithReport(c, source, rep))
		if c.NextSibling() != nil && isBlockNode(c) {
			b.WriteString("\n")
		}
	}
	return b.String()
}

// escapeText prevents the resulting markdown from creating links/images/autolinks/etc.
func escapeText(s string) string {
	s = strings.ReplaceAll(s, "\x00", "")

	var buf bytes.Buffer
	buf.Grow(len(s))

	for _, r := range s {
		switch r {
		case '\\', '*', '_', '[', ']', '(', ')', '!', '`':
			buf.WriteByte('\\')
			buf.WriteRune(r)
		case '<', '>':
			buf.WriteByte('\\')
			buf.WriteRune(r)
		default:
			buf.WriteRune(r)
		}
	}
	return buf.String()
}

// isBlockNode performs its package-specific operation.
func isBlockNode(n ast.Node) bool {
	switch n.(type) {
	case *ast.Paragraph, *ast.Blockquote:
		return true
	default:
		return false
	}
}
