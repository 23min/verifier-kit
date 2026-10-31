// Package shlex splits and quotes command lines by the shell's quoting rules: the toolkit's
// one way to split a command line into words, shared by spine, for conform --adapter-cmd, and
// verify, for every .cmd file.
package shlex

import (
	"errors"
	"strings"
)

// Split splits s into words by the POSIX shell's quoting rules, without comments, variables or
// globs: words are separated by spaces, tabs and line breaks; outside quotes a backslash keeps the
// next character, and joins lines when that is a line break; single quotes keep everything; in
// double quotes a backslash keeps only $, `, ", \ or a line break, which it joins. Its special
// characters are ASCII, so it reads bytes, and a byte that isn't UTF-8 reaches the program as it
// was given.
func Split(s string) ([]string, error) {
	var out []string
	var word []byte
	inWord := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			if inWord {
				out, word, inWord = append(out, string(word)), nil, false
			}
		case c == '\\':
			if i+1 == len(s) {
				return nil, errors.New("no escaped character")
			}
			i++
			if s[i] != '\n' {
				word, inWord = append(word, s[i]), true
			}
		case c == '\'':
			end := strings.IndexByte(s[i+1:], '\'')
			if end < 0 {
				return nil, errors.New("no closing quotation")
			}
			word, inWord = append(word, s[i+1:i+1+end]...), true
			i += end + 1
		case c == '"':
			text, end, err := doubleQuoted(s, i+1)
			if err != nil {
				return nil, err
			}
			word, inWord, i = append(word, text...), true, end
		default:
			word, inWord = append(word, c), true
		}
	}
	if inWord {
		out = append(out, string(word))
	}
	return out, nil
}

// doubleQuoted reads s from i up to its closing double quote: the text, and where the quote is.
func doubleQuoted(s string, i int) ([]byte, int, error) {
	var text []byte
	for ; i < len(s); i++ {
		switch c := s[i]; {
		case c == '"':
			return text, i, nil
		case c == '\\' && i+1 < len(s) && strings.IndexByte("$`\"\\\n", s[i+1]) >= 0:
			if i++; s[i] != '\n' {
				text = append(text, s[i])
			}
		default:
			text = append(text, c)
		}
	}
	return nil, 0, errors.New("no closing quotation")
}

// Quote is s as one word of a command line.
func Quote(s string) string {
	if s != "" && strings.Trim(s, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_@%+=:,./-") == "" {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}
