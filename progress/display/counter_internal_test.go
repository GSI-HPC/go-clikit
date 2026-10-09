// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package display

import (
	"testing"
	"time"
)

// clock reads the time of a live display in minutes, seconds and tenths,
// and in hours, minutes, seconds and tenths from an hour on. The tenths are
// cut, not rounded, so that it never runs ahead of the time: 999ms is still
// 0.9s, and 3599.99s is still short of the hour. A negative duration, of a
// clock that went back, reads as none.
func TestClock(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		d    time.Duration
		want string
	}{
		{-time.Hour, "0:00.0"},
		{-time.Nanosecond, "0:00.0"},
		{0, "0:00.0"},
		{99 * time.Millisecond, "0:00.0"},
		{100 * time.Millisecond, "0:00.1"},
		{999 * time.Millisecond, "0:00.9"},
		{time.Second, "0:01.0"},
		{41300 * time.Millisecond, "0:41.3"},
		{59950 * time.Millisecond, "0:59.9"},
		{time.Minute - time.Nanosecond, "0:59.9"},
		{time.Minute, "1:00.0"},
		{4*time.Minute + 12600*time.Millisecond, "4:12.6"},
		{3599990 * time.Millisecond, "59:59.9"},
		{time.Hour, "1:00:00.0"},
		{time.Hour + 2*time.Minute + 3450*time.Millisecond, "1:02:03.4"},
		{100*time.Hour + 59*time.Second, "100:00:59.0"},
	} {
		if got := clock(tc.d); got != tc.want {
			t.Errorf("clock(%v) = %q, want %q", tc.d, got, tc.want)
		}
	}
}

// elapsed reads the time in front of a plain line in whole seconds, as a log
// keeps it: minutes and seconds, and hours, minutes and seconds from an hour
// on.
func TestElapsed(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		d    time.Duration
		want string
	}{
		{0, "0:00"},
		{3999 * time.Millisecond, "0:03"},
		{time.Hour - time.Nanosecond, "59:59"},
		{time.Hour, "1:00:00"},
		{time.Hour + 2*time.Minute + 3999*time.Millisecond, "1:02:03"},
	} {
		if got := elapsed(tc.d); got != tc.want {
			t.Errorf("elapsed(%v) = %q, want %q", tc.d, got, tc.want)
		}
	}
}
