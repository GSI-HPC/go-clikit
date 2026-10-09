// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package display_test

import (
	"flag"
	"io"
	"slices"
	"testing"

	"github.com/GSI-HPC/go-clikit/progress/display"
)

// Every theme has a name of its own, which ParseTheme reads back to the
// theme, in any case; the zero Theme is "none".
func TestThemesAreNamed(t *testing.T) {
	t.Parallel()
	var names []string
	for _, theme := range display.Themes() {
		names = append(names, theme.String())
		for _, name := range []string{theme.String(), string(theme.String()[0]-'a'+'A') + theme.String()[1:]} {
			got, err := display.ParseTheme(name)
			if err != nil || got != theme {
				t.Errorf("ParseTheme(%q) = %v, %v; want %v", name, got, err, theme)
			}
		}
	}
	if want := []string{"classic", "aurora", "ember", "neon", "tide"}; !slices.Equal(names, want) {
		t.Errorf("the themes are %v, want %v", names, want)
	}
	if got := (display.Theme{}).String(); got != "none" {
		t.Errorf("the zero Theme is named %q, want none", got)
	}
	// The slice is the caller's.
	display.Themes()[0] = display.Tide
	if display.Themes()[0] != display.Classic {
		t.Error("changing the slice Themes returned changed the next")
	}
}

// ParseTheme reads "none" and "" as the zero Theme, and any name no theme
// has as an error that quotes it as it was given and lists the names.
func TestParseTheme(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		want display.Theme
		err  string
	}{
		{"", display.Theme{}, ""},
		{"none", display.Theme{}, ""},
		{"NONE", display.Theme{}, ""},
		{"Tide", display.Tide, ""},
		{"auora", display.Theme{}, `display: unknown theme "auora": want none, classic, aurora, ember, neon or tide`},
		{"Dawn", display.Theme{}, `display: unknown theme "Dawn": want none, classic, aurora, ember, neon or tide`},
	} {
		got, err := display.ParseTheme(tc.name)
		if got != tc.want {
			t.Errorf("ParseTheme(%q) = %v, want %v", tc.name, got, tc.want)
		}
		if gotErr := errText(err); gotErr != tc.err {
			t.Errorf("ParseTheme(%q) failed with %q, want %q", tc.name, gotErr, tc.err)
		}
	}
}

// errText is err's text, "" for none.
func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// In draws a theme in so many colours, and leaves the zero Theme as it is,
// which draws in none; a theme compares equal to itself in the same
// colours, and not in others.
func TestThemeIn(t *testing.T) {
	t.Parallel()
	if got := (display.Theme{}).In(display.Colours16); got != (display.Theme{}) {
		t.Errorf("the zero Theme in 16 colours is %v, want the zero Theme", got)
	}
	for _, c := range []display.Colours{display.Colours256, display.Colours16} {
		if got := (display.Theme{}).In(c).Colours(); got != display.NoColours {
			t.Errorf("the zero Theme in %v colours draws in %v, want none", c, got)
		}
	}
	if display.Aurora.In(display.Colours256) != display.Aurora {
		t.Error("Aurora in 256 colours is not Aurora, which is drawn in 256")
	}
	sixteen := display.Aurora.In(display.Colours16)
	if sixteen == display.Aurora || sixteen.Colours() != display.Colours16 || sixteen.String() != "aurora" {
		t.Errorf("Aurora in 16 colours is %v in %v", sixteen, sixteen.Colours())
	}
	if display.Aurora.Colours() != display.Colours256 {
		t.Errorf("Aurora is drawn in %v colours, want 256", display.Aurora.Colours())
	}
}

// Colours names how many colours it draws in.
func TestColoursString(t *testing.T) {
	t.Parallel()
	for c, want := range map[display.Colours]string{
		display.Colours256: "256", display.Colours16: "16", display.NoColours: "none", 7: "Colours(7)",
	} {
		if got := c.String(); got != want {
			t.Errorf("Colours(%d) is %q, want %q", c, got, want)
		}
	}
}

// A Theme is the value of a flag through its text methods, which keep the
// colours of the theme it is drawn in and leave it as it was for a name no
// theme has. The zero Theme keeps no colours: a theme read over it is drawn
// in 256.
func TestAThemeIsAFlag(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		value display.Theme
		args  []string
		want  display.Theme
		err   bool
	}{
		{display.Classic, nil, display.Classic, false},
		{display.Classic, []string{"-theme", "Tide"}, display.Tide, false},
		{display.Classic, []string{"-theme=none"}, display.Theme{}, false},
		{display.Classic, []string{"-theme=dawn"}, display.Classic, true},
		{display.Classic.In(display.NoColours), []string{"-theme", "aurora"}, display.Aurora.In(display.NoColours), false},
		{display.Theme{}.In(display.NoColours), []string{"-theme", "aurora"}, display.Aurora, false},
	} {
		fs := flag.NewFlagSet("prog", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		var theme display.Theme
		fs.TextVar(&theme, "theme", tc.value, "how progress looks")
		err := fs.Parse(tc.args)
		if theme != tc.want || (err != nil) != tc.err {
			t.Errorf("%v over %v in %v: the flag reads %v in %v, %v; want %v in %v, an error %v",
				tc.args, tc.value, tc.value.Colours(), theme, theme.Colours(), err, tc.want, tc.want.Colours(), tc.err)
		}
	}
	theme := display.Ember.In(display.Colours16)
	if err := theme.UnmarshalText([]byte("neon")); err != nil || theme != display.Neon.In(display.Colours16) {
		t.Errorf("UnmarshalText of neon over Ember in 16 colours gives %v in %v, %v; want Neon in 16", theme, theme.Colours(), err)
	}
	if text, err := display.Neon.MarshalText(); string(text) != "neon" || err != nil {
		t.Errorf("MarshalText of Neon = %q, %v", text, err)
	}
}
