// Copyright 2020 The go-zeromq Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package zyre provides reliable group messaging over local area networks.
//
// For more informations, see https://zeromq.org.
package zyre

import "testing"

func TestEventHeader(t *testing.T) {
	tests := []struct {
		name    string
		headers map[string]string
		value   string
		ok      bool
	}{
		{"HEADER", map[string]string{"HEADER": "v"}, "v", true},
		{"NOHEADER", map[string]string{"HEADER": "v"}, "v", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := &Event{
				Headers: tt.headers,
			}
			value, ok := e.Header(tt.name)
			if ok == true && value != tt.value {
				t.Errorf("Event.Header() value got = %v, want %v", value, tt.value)
			}
			if ok != tt.ok {
				t.Errorf("Event.Header() ok got = %v, want %v", ok, tt.ok)
			}
		})
	}
}
