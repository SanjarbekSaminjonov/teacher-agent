package bot

import (
	"fmt"
	"html"
	"regexp"
	"strings"
)

var (
	codeBlockRegex  = regexp.MustCompile("(?s)```([a-zA-Z0-9_-]*)\n?(.*?)```")
	inlineCodeRegex = regexp.MustCompile("`([^`\n]+)`")
	boldRegex       = regexp.MustCompile(`\*\*([^*]+?)\*\*`)
	strikeRegex     = regexp.MustCompile(`~~([^~]+?)~~`)
)

// MarkdownToTelegramHTML converts markdown into valid, safe Telegram HTML.
func MarkdownToTelegramHTML(md string) string {
	if strings.TrimSpace(md) == "" {
		return ""
	}

	placeholderMap := make(map[string]string)
	pIdx := 0

	// 1. Extract and protect multi-line code blocks
	text := codeBlockRegex.ReplaceAllStringFunc(md, func(match string) string {
		sub := codeBlockRegex.FindStringSubmatch(match)
		lang := ""
		code := match
		if len(sub) >= 3 {
			lang = strings.TrimSpace(sub[1])
			code = sub[2]
		} else if len(sub) == 2 {
			code = sub[1]
		}
		escapedCode := html.EscapeString(code)
		var formatted string
		if lang != "" {
			formatted = fmt.Sprintf("<pre><code class=\"language-%s\">%s</code></pre>", lang, escapedCode)
		} else {
			formatted = fmt.Sprintf("<pre>%s</pre>", escapedCode)
		}

		key := fmt.Sprintf("\x00BLOCK_%d\x00", pIdx)
		pIdx++
		placeholderMap[key] = formatted
		return key
	})

	// 2. Extract and protect inline code `...`
	text = inlineCodeRegex.ReplaceAllStringFunc(text, func(match string) string {
		sub := inlineCodeRegex.FindStringSubmatch(match)
		code := match
		if len(sub) >= 2 {
			code = sub[1]
		}
		escapedCode := html.EscapeString(code)
		formatted := fmt.Sprintf("<code>%s</code>", escapedCode)

		key := fmt.Sprintf("\x00INLINE_%d\x00", pIdx)
		pIdx++
		placeholderMap[key] = formatted
		return key
	})

	// 3. Escape HTML special characters in the remaining text
	text = html.EscapeString(text)

	// 4. Handle Blockquotes (lines starting with &gt; because > was escaped in step 3)
	lines := strings.Split(text, "\n")
	var processedLines []string
	var quoteBuffer []string

	flushQuote := func() {
		if len(quoteBuffer) > 0 {
			content := strings.Join(quoteBuffer, "\n")
			processedLines = append(processedLines, fmt.Sprintf("<blockquote>%s</blockquote>", content))
			quoteBuffer = nil
		}
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "&gt;") {
			var qLine string
			if strings.HasPrefix(trimmed, "&gt; ") {
				qLine = strings.TrimPrefix(trimmed, "&gt; ")
			} else {
				qLine = strings.TrimPrefix(trimmed, "&gt;")
			}
			quoteBuffer = append(quoteBuffer, qLine)
		} else {
			flushQuote()
			processedLines = append(processedLines, line)
		}
	}
	flushQuote()
	text = strings.Join(processedLines, "\n")

	// 5. Bold: **text** -> <b>text</b>
	text = boldRegex.ReplaceAllString(text, "<b>$1</b>")

	// 6. Strikethrough: ~~text~~ -> <s>text</s>
	text = strikeRegex.ReplaceAllString(text, "<s>$1</s>")

	// 7. Italic: *text* -> <i>text</i> (single asterisk, but not inside tags)
	italicRegex := regexp.MustCompile(`(^|[^*])\*([^*\n]+?)\*([^*]|$)`)
	text = italicRegex.ReplaceAllString(text, "${1}<i>${2}</i>${3}")

	// 8. Restore code placeholders
	for key, val := range placeholderMap {
		text = strings.ReplaceAll(text, key, val)
	}

	return text
}
