// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package progress

import (
	"reflect"
	"strings"
	"testing"
)

// Fields is embedded in Event, and the event log writes both flat, on one
// line: a member of Fields named as one of Event would be shadowed without
// a word, and a key of the log used twice would hide one of its values.
// Neither may happen.
func TestEventAndFieldsShareNoName(t *testing.T) {
	t.Parallel()
	names := map[string]string{}
	for _, typ := range []reflect.Type{reflect.TypeFor[Event](), reflect.TypeFor[Fields]()} {
		for i := range typ.NumField() {
			f := typ.Field(i)
			if f.Anonymous {
				continue
			}
			if other, ok := names[f.Name]; ok {
				t.Errorf("%s.%s has the name of %s.%s", typ.Name(), f.Name, other, f.Name)
			}
			names[f.Name] = typ.Name()
		}
	}

	keys := map[string]string{}
	line := reflect.TypeFor[logEvent]()
	for i := range line.NumField() {
		f := line.Field(i)
		key, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		if key == "" || key == "-" {
			t.Errorf("logEvent.%s has no key of its own", f.Name)
			continue
		}
		if other, ok := keys[key]; ok {
			t.Errorf("logEvent.%s and logEvent.%s are both logged as %q", f.Name, other, key)
		}
		keys[key] = f.Name
	}
}
