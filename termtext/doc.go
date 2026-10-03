// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

// Package termtext makes untrusted text safe to write to a terminal, and
// measures how many columns text takes there.
//
// Text from elsewhere, such as the output of a program on a remote host or
// in a container, an error it returned, or a value a service reported, is
// not trusted: a carriage return or a cursor movement can overwrite a line
// printed before, an escape sequence can retitle the terminal or write the
// clipboard, and a bidirectional control can show text in another order
// than it has. Escape and EscapeLines show such characters as visible
// escapes instead. A program that escapes everything with this package, and
// nothing with anything else, has one place to fix when a character turns
// out to need escaping too.
//
// # Escape policy
//
// The policy is a deny-list: a rune is escaped when it can move the cursor,
// change the state of the terminal, or change the order or the lines text
// is shown in, and is written as it is otherwise. Escaped are:
//
//   - every C0 control (U+0000 to U+001F) and DEL (U+007F), except that
//     EscapeLines keeps newline and tab, which cannot move the cursor back
//     over text already written;
//   - every C1 control (U+0080 to U+009F), which some terminals read as the
//     introducer of a control sequence;
//   - the bidirectional controls U+061C, U+200E, U+200F, U+202A to U+202E
//     and U+2066 to U+2069;
//   - the line and paragraph separators U+2028 and U+2029;
//   - every byte that is not part of valid UTF-8, such as a lone 0x9b.
//
// Other invisible characters, such as a zero-width space, a byte order mark
// or a tag character, are written as they are: they change neither the
// cursor nor the terminal, and an allow-list that escaped them would escape
// text in scripts that need joiners and marks too. A caller that compares
// names should not rely on this package to make look-alikes visible.
//
// The escapes are for a reader, not for decoding: a backslash is written as
// it is, so text that holds the four characters \x1b looks the same as text
// whose ESC was escaped, and the original cannot always be recovered from
// what is shown. Escaping the backslash too would double every backslash
// in a path or a pattern, and would make escaped text change each time it
// is escaped again, which the callers of Escape rely on it not to. A
// program that has to tell the two apart keeps the text as it came, not
// escaped.
//
// # Widths
//
// RuneWidth and Width approximate the columns a terminal gives text. East
// Asian wide and fullwidth characters take two columns. Combining marks and
// format characters take none, save the soft hyphen, the prepended
// concatenation marks such as U+0600, and the Ahom medial ra U+1171E (a
// spacing mark since Unicode 16.0), which are drawn and take one column. The
// rule for marks comes first, so a combining mark that is East Asian wide,
// such as the ideographic tone marks U+302A to U+302D, takes none as well.
// NUL takes none, and every other character one. Width counts a character
// followed by the variation selector U+FE0F, which asks for its emoji
// picture, as two columns, as terminals draw it. Beyond that it does not
// cluster graphemes, so a sequence joined with U+200D, a skin tone modifier
// or a flag is counted as the sum of its runes, which may be more than a
// terminal shows. The widths are meant never to be less than a terminal
// shows, so that a row Truncate cut does not wrap, but they follow Unicode
// up to version 18.0: a wide character assigned later is counted as one
// column, and a terminal that draws U+FE0F narrow shows less than Width
// counts. Truncate shortens text to a number of columns with the same
// widths.
package termtext
