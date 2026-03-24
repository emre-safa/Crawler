package index

import (
	"strings"
	"unicode"
)

// Tokenize splits text into lowercase tokens, stripping punctuation and stop words.
func Tokenize(text string) []string {
	words := strings.FieldsFunc(text, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	tokens := make([]string, 0, len(words))
	for _, w := range words {
		w = strings.ToLower(w)
		if len(w) > 0 && !isStopWord(w) {
			tokens = append(tokens, w)
		}
	}
	return tokens
}

var stopWords = map[string]bool{
	"a": true, "an": true, "and": true, "are": true, "as": true, "at": true,
	"be": true, "by": true, "for": true, "from": true, "has": true, "he": true,
	"in": true, "is": true, "it": true, "its": true, "of": true, "on": true,
	"or": true, "that": true, "the": true, "to": true, "was": true, "were": true,
	"will": true, "with": true, "this": true, "but": true, "they": true,
	"have": true, "had": true, "what": true, "when": true, "where": true,
	"who": true, "which": true, "their": true, "if": true, "each": true,
	"how": true, "she": true, "do": true, "not": true, "did": true,
}

func isStopWord(w string) bool {
	return stopWords[w]
}
