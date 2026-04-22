//go:build !cgo

package types

import (
	"strings"
	"unicode"
)

type fallbackWordSegmenter struct{}

func newWordSegmenter() WordSegmenter {
	return fallbackWordSegmenter{}
}

func (fallbackWordSegmenter) Cut(text string, _ bool) []string {
	return splitTokens(text, false)
}

func (fallbackWordSegmenter) CutForSearch(text string, _ bool) []string {
	return splitTokens(text, true)
}

func splitTokens(text string, forSearch bool) []string {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) == 0 {
		return nil
	}

	var tokens []string
	var latin []rune
	flushLatin := func() {
		if len(latin) == 0 {
			return
		}
		tokens = append(tokens, strings.ToLower(string(latin)))
		latin = latin[:0]
	}

	for _, r := range runes {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			if r <= unicode.MaxASCII {
				latin = append(latin, unicode.ToLower(r))
				continue
			}
			flushLatin()
			tokens = append(tokens, string(r))
		default:
			flushLatin()
		}
	}
	flushLatin()

	if !forSearch {
		return tokens
	}

	expanded := make([]string, 0, len(tokens)*2)
	for i, tok := range tokens {
		expanded = append(expanded, tok)
		if len([]rune(tok)) == 1 && i+1 < len(tokens) && len([]rune(tokens[i+1])) == 1 {
			expanded = append(expanded, tok+tokens[i+1])
		}
	}
	return expanded
}
