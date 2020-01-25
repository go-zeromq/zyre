// Copyright 2020 The go-zeromq Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package zyre provides reliable group messaging over local area networks.
//
// For more informations, see https://zeromq.org.
package zyre

// Event zyre event
type Event struct {
	Type     string            // Event type
	PeerUUID string            // Sender UUID as string
	PeerName string            // Sender public name as string
	PeerAddr string            // Sender ipaddress as string, for an ENTER event
	Headers  map[string]string // Headers, for an ENTER event
	Group    string            // Group name for a SHOUT event
	Msg      []byte            // Message payload for SHOUT or WHISPER
}

// Header returns value of a header from the message headers
// obtained by ENTER.
func (e *Event) Header(name string) (string, bool) {
	v, ok := e.Headers[name]
	return v, ok
}
