package tui

import (
	"mime"
	"strings"
	"unicode/utf8"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"goman/internal/gocui"
)

const maxHighlightBytes = 64 << 10

func syntaxTokens(field, text string) []chroma.Token {
	if field == "url" {
		return []chroma.Token{{Type: chroma.Text, Value: text}}
	}
	prefix, body := "", text
	if field == "response" || field == "hook-detail" {
		if split := strings.Index(text, "\n\n"); split >= 0 {
			prefix, body = text[:split+2], text[split+2:]
		}
	}
	var tokens []chroma.Token
	for _, line := range strings.SplitAfter(prefix, "\n") {
		if colon := strings.IndexByte(line, ':'); colon > 0 {
			tokens = append(tokens, chroma.Token{Type: chroma.NameAttribute, Value: line[:colon]}, chroma.Token{Type: chroma.Text, Value: line[colon:]})
		} else {
			tokens = append(tokens, chroma.Token{Type: chroma.Text, Value: line})
		}
	}
	language := ""
	trimmed := strings.TrimSpace(body)
	if field == "headers" || strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		language = "JSON"
	}
	if strings.HasPrefix(trimmed, "<") {
		language = "XML"
		if strings.HasPrefix(strings.ToLower(trimmed), "<!doctype html") || strings.HasPrefix(strings.ToLower(trimmed), "<html") {
			language = "HTML"
		}
	}
	for _, line := range strings.Split(prefix, "\n") {
		key, value, found := strings.Cut(line, ":")
		if !found || !strings.EqualFold(key, "Content-Type") {
			continue
		}
		mediaType, _, err := mime.ParseMediaType(strings.TrimSpace(value))
		if err != nil {
			continue
		}
		switch {
		case mediaType == "application/json" || strings.HasSuffix(mediaType, "+json"):
			language = "JSON"
		case strings.Contains(mediaType, "yaml"):
			language = "YAML"
		case mediaType == "text/html":
			language = "HTML"
		case strings.Contains(mediaType, "xml"):
			language = "XML"
		}
	}
	if language == "" && (strings.HasPrefix(trimmed, "---\n") || strings.HasPrefix(trimmed, "- ")) {
		language = "YAML"
	}
	lexer := lexers.Get(language)
	if language == "" || lexer == nil {
		return append(tokens, chroma.Token{Type: chroma.Text, Value: body})
	}
	remainder := ""
	if len(body) > maxHighlightBytes {
		end := maxHighlightBytes
		for end > 0 && !utf8.RuneStart(body[end]) {
			end--
		}
		body, remainder = body[:end], body[end:]
	}
	iterator, err := chroma.Coalesce(lexer).Tokenise(&chroma.TokeniseOptions{State: "root"}, body)
	if err != nil {
		return append(tokens, chroma.Token{Type: chroma.Text, Value: body + remainder})
	}
	result := iterator.Tokens()
	var source strings.Builder
	for _, token := range result {
		source.WriteString(token.Value)
	}
	if source.String() != body {
		return append(tokens, chroma.Token{Type: chroma.Text, Value: body + remainder})
	}
	tokens = append(tokens, result...)
	if remainder != "" {
		tokens = append(tokens, chroma.Token{Type: chroma.Text, Value: remainder})
	}
	return tokens
}

func tokenColor(kind chroma.TokenType) string {
	switch {
	case kind == chroma.NameTag || kind == chroma.NameAttribute:
		return "\x1b[38;2;100;200;240m"
	case kind.InSubCategory(chroma.LiteralString):
		return "\x1b[38;2;140;210;150m"
	case kind.InSubCategory(chroma.LiteralNumber):
		return "\x1b[38;2;240;190;100m"
	case kind.InCategory(chroma.Keyword):
		return "\x1b[38;2;230;140;180m"
	case kind.InCategory(chroma.Comment):
		return "\x1b[38;2;145;150;155m"
	case kind.InCategory(chroma.Punctuation) || kind.InCategory(chroma.Operator):
		return "\x1b[38;2;195;200;205m"
	default:
		return ""
	}
}

func renderSyntax(view *gocui.View, text string, start, end int) {
	renderTokens(view, syntaxTokens(view.Name(), text), start, end)
}

func renderTokens(view *gocui.View, tokens []chroma.Token, start, end int) {
	var output strings.Builder
	offset := 0
	for _, token := range tokens {
		value := token.Value
		for len(value) > 0 {
			length := len(value)
			if start > offset {
				length = min(length, start-offset)
			}
			if end > offset {
				length = min(length, end-offset)
			}
			output.WriteString(tokenColor(token.Type))
			if start <= offset && offset < end {
				output.WriteString("\x1b[7m")
			}
			output.WriteString(plain(value[:length]))
			output.WriteString("\x1b[0m")
			value = value[length:]
			offset += length
		}
	}
	view.SetContent(output.String())
}
