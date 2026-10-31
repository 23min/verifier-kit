// Package shlex reads and writes command lines as Python's shlex module does: the toolkit's
// one way to split a command line into words, shared by spine, for conform --adapter-cmd, and
// verify, for every .cmd file.
package shlex

import (
	"errors"
	"strings"
)

// Split is shlex.split(s): POSIX rules, no comments. Its special characters are ASCII, so
// it reads bytes, and a byte that isn't UTF-8 reaches the program as it was given.
func Split(s string) ([]string, error) {
	const whitespace, quotes = " \t\r\n", `'"`
	pos := 0
	var out []string
	for {
		var token []byte
		state, escapedState, quoted, any := rune(' '), rune('a'), false, false
	read:
		for {
			var next rune = -1
			if pos < len(s) {
				next = rune(s[pos])
				pos++
			}
			switch {
			case state == ' ':
				switch {
				case next < 0:
					break read
				case strings.ContainsRune(whitespace, next):
					if any || quoted {
						break read
					}
				case next == '\\':
					escapedState, state = 'a', '\\'
				case strings.ContainsRune(quotes, next):
					state = next
				default:
					token = append(token, byte(next))
					any, state = true, 'a'
				}
			case state == '\'' || state == '"':
				quoted = true
				switch {
				case next < 0:
					return nil, errors.New("no closing quotation")
				case next == state:
					state = 'a'
				case next == '\\' && state == '"':
					escapedState, state = state, '\\'
				default:
					token = append(token, byte(next))
					any = true
				}
			case state == '\\':
				if next < 0 {
					return nil, errors.New("no escaped character")
				}
				if (escapedState == '\'' || escapedState == '"') && next != state && next != escapedState {
					token = append(token, byte(state))
				}
				token = append(token, byte(next))
				any, state = true, escapedState
			default: // 'a', inside a word
				switch {
				case next < 0:
					break read
				case strings.ContainsRune(whitespace, next):
					break read
				case strings.ContainsRune(quotes, next):
					state = next
				case next == '\\':
					escapedState, state = 'a', '\\'
				default:
					token = append(token, byte(next))
					any = true
				}
			}
		}
		if !any && !quoted {
			return out, nil
		}
		out = append(out, string(token))
	}
}

// Quote is shlex.quote(s): s as one word of a command line.
func Quote(s string) string {
	if s != "" && strings.Trim(s, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_@%+=:,./-") == "" {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}
