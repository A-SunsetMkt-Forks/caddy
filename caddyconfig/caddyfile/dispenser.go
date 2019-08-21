// Copyright 2015 Matthew Holt and The Caddy Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package caddyfile

import (
	"errors"
	"fmt"
	"strings"
)

// Dispenser is a type that dispenses tokens, similarly to a lexer,
// except that it can do so with some notion of structure. An empty
// Dispenser is invalid; call NewDispenser to make a proper instance.
type Dispenser struct {
	tokens  []Token
	cursor  int
	nesting int
}

// NewDispenser returns a Dispenser filled with the given tokens.
func NewDispenser(tokens []Token) *Dispenser {
	return &Dispenser{
		tokens: tokens,
		cursor: -1,
	}
}

// Next loads the next token. Returns true if a token
// was loaded; false otherwise. If false, all tokens
// have been consumed.
func (d *Dispenser) Next() bool {
	if d.cursor < len(d.tokens)-1 {
		d.cursor++
		return true
	}
	return false
}

// Prev moves to the previous token. It does the inverse
// of Next(), except this function may decrement the cursor
// to -1 so that the next call to Next() points to the
// first token; this allows dispensing to "start over". This
// method returns true if the cursor ends up pointing to a
// valid token.
func (d *Dispenser) Prev() bool {
	if d.cursor > -1 {
		d.cursor--
		return d.cursor > -1
	}
	return false
}

// NextArg loads the next token if it is on the same
// line and if it is not a block opening (open curly
// brace). Returns true if an argument token was
// loaded; false otherwise. If false, all tokens on
// the line have been consumed except for potentially
// a block opening. It handles imported tokens
// correctly.
func (d *Dispenser) NextArg() bool {
	if !d.nextOnSameLine() {
		return false
	}
	if d.Val() == "{" {
		// roll back; a block opening is not an argument
		d.cursor--
		return false
	}
	return true
}

// nextOnSameLine advances the cursor if the next
// token is on the same line of the same file.
func (d *Dispenser) nextOnSameLine() bool {
	if d.cursor < 0 {
		d.cursor++
		return true
	}
	if d.cursor >= len(d.tokens) {
		return false
	}
	if d.cursor < len(d.tokens)-1 &&
		d.tokens[d.cursor].File == d.tokens[d.cursor+1].File &&
		d.tokens[d.cursor].Line+d.numLineBreaks(d.cursor) == d.tokens[d.cursor+1].Line {
		d.cursor++
		return true
	}
	return false
}

// NextLine loads the next token only if it is not on the same
// line as the current token, and returns true if a token was
// loaded; false otherwise. If false, there is not another token
// or it is on the same line. It handles imported tokens correctly.
func (d *Dispenser) NextLine() bool {
	if d.cursor < 0 {
		d.cursor++
		return true
	}
	if d.cursor >= len(d.tokens) {
		return false
	}
	if d.cursor < len(d.tokens)-1 &&
		(d.tokens[d.cursor].File != d.tokens[d.cursor+1].File ||
			d.tokens[d.cursor].Line+d.numLineBreaks(d.cursor) < d.tokens[d.cursor+1].Line) {
		d.cursor++
		return true
	}
	return false
}

// NextBlock can be used as the condition of a for loop
// to load the next token as long as it opens a block or
// is already in a block. It returns true if a token was
// loaded, or false when the block's closing curly brace
// was loaded and thus the block ended. Nested blocks are
// not supported.
func (d *Dispenser) NextBlock() bool {
	if d.nesting > 0 {
		d.Next()
		if d.Val() == "}" {
			d.nesting--
			return false
		}
		return true
	}
	if !d.nextOnSameLine() { // block must open on same line
		return false
	}
	if d.Val() != "{" {
		d.cursor-- // roll back if not opening brace
		return false
	}
	d.Next()
	if d.Val() == "}" {
		// open and then closed right away
		return false
	}
	d.nesting++
	return true
}

// Nested returns true if the token is currently nested
// inside a block (i.e. an open curly brace was consumed).
func (d *Dispenser) Nested() bool {
	return d.nesting > 0
}

// Val gets the text of the current token. If there is no token
// loaded, it returns empty string.
func (d *Dispenser) Val() string {
	if d.cursor < 0 || d.cursor >= len(d.tokens) {
		return ""
	}
	return d.tokens[d.cursor].Text
}

// Line gets the line number of the current token.
// If there is no token loaded, it returns 0.
func (d *Dispenser) Line() int {
	if d.cursor < 0 || d.cursor >= len(d.tokens) {
		return 0
	}
	return d.tokens[d.cursor].Line
}

// File gets the filename where the current token originated.
func (d *Dispenser) File() string {
	if d.cursor < 0 || d.cursor >= len(d.tokens) {
		return ""
	}
	return d.tokens[d.cursor].File
}

// Args is a convenience function that loads the next arguments
// (tokens on the same line) into an arbitrary number of strings
// pointed to in targets. If there are fewer tokens available
// than string pointers, the remaining strings will not be changed
// and false will be returned. If there were enough tokens available
// to fill the arguments, then true will be returned.
func (d *Dispenser) Args(targets ...*string) bool {
	for i := 0; i < len(targets); i++ {
		if !d.NextArg() {
			return false
		}
		*targets[i] = d.Val()
	}
	return true
}

// RemainingArgs loads any more arguments (tokens on the same line)
// into a slice and returns them. Open curly brace tokens also indicate
// the end of arguments, and the curly brace is not included in
// the return value nor is it loaded.
func (d *Dispenser) RemainingArgs() []string {
	var args []string
	for d.NextArg() {
		args = append(args, d.Val())
	}
	return args
}

// NewFromNextTokens returns a new dispenser with a copy of
// the tokens from the current token until the end of the
// "directive" whether that be to the end of the line or
// the end of a block that starts at the end of the line.
func (d *Dispenser) NewFromNextTokens() *Dispenser {
	tkns := []Token{d.Token()}
	for d.NextArg() {
		tkns = append(tkns, d.Token())
	}
	if d.Next() && d.Val() == "{" {
		tkns = append(tkns, d.Token())
		for d.NextBlock() {
			for d.Nested() {
				tkns = append(tkns, d.Token())
				d.NextBlock()
			}
		}
		tkns = append(tkns, d.Token())
	} else {
		d.cursor--
	}
	return NewDispenser(tkns)
}

// Token returns the current token.
func (d *Dispenser) Token() Token {
	if d.cursor < 0 || d.cursor >= len(d.tokens) {
		return Token{}
	}
	return d.tokens[d.cursor]
}

// Reset sets d's cursor to the beginning, as
// if this was a new and unused dispenser.
func (d *Dispenser) Reset() {
	d.cursor = -1
	d.nesting = 0
}

// ArgErr returns an argument error, meaning that another
// argument was expected but not found. In other words,
// a line break or open curly brace was encountered instead of
// an argument.
func (d *Dispenser) ArgErr() error {
	if d.Val() == "{" {
		return d.Err("Unexpected token '{', expecting argument")
	}
	return d.Errf("Wrong argument count or unexpected line ending after '%s'", d.Val())
}

// SyntaxErr creates a generic syntax error which explains what was
// found and what was expected.
func (d *Dispenser) SyntaxErr(expected string) error {
	msg := fmt.Sprintf("%s:%d - Syntax error: Unexpected token '%s', expecting '%s'", d.File(), d.Line(), d.Val(), expected)
	return errors.New(msg)
}

// EOFErr returns an error indicating that the dispenser reached
// the end of the input when searching for the next token.
func (d *Dispenser) EOFErr() error {
	return d.Errf("Unexpected EOF")
}

// Err generates a custom parse-time error with a message of msg.
func (d *Dispenser) Err(msg string) error {
	msg = fmt.Sprintf("%s:%d - Error during parsing: %s", d.File(), d.Line(), msg)
	return errors.New(msg)
}

// Errf is like Err, but for formatted error messages
func (d *Dispenser) Errf(format string, args ...interface{}) error {
	return d.Err(fmt.Sprintf(format, args...))
}

// Delete deletes the current token and returns the updated slice
// of tokens. The cursor is not advanced to the next token.
// Because deletion modifies the underlying slice, this method
// should only be called if you have access to the original slice
// of tokens and/or are using the slice of tokens outside this
// Dispenser instance. If you do not re-assign the slice with the
// return value of this method, inconsistencies in the token
// array will become apparent (or worse, hide from you like they
// did me for 3 and a half freaking hours late one night).
func (d *Dispenser) Delete() []Token {
	if d.cursor >= 0 && d.cursor < len(d.tokens)-1 {
		d.tokens = append(d.tokens[:d.cursor], d.tokens[d.cursor+1:]...)
		d.cursor--
	}
	return d.tokens
}

// numLineBreaks counts how many line breaks are in the token
// value given by the token index tknIdx. It returns 0 if the
// token does not exist or there are no line breaks.
func (d *Dispenser) numLineBreaks(tknIdx int) int {
	if tknIdx < 0 || tknIdx >= len(d.tokens) {
		return 0
	}
	return strings.Count(d.tokens[tknIdx].Text, "\n")
}

// isNewLine determines whether the current token is on a different
// line (higher line number) than the previous token. It handles imported
// tokens correctly. If there isn't a previous token, it returns true.
func (d *Dispenser) isNewLine() bool {
	if d.cursor < 1 {
		return true
	}
	if d.cursor > len(d.tokens)-1 {
		return false
	}
	return d.tokens[d.cursor-1].File != d.tokens[d.cursor].File ||
		d.tokens[d.cursor-1].Line+d.numLineBreaks(d.cursor-1) < d.tokens[d.cursor].Line
}