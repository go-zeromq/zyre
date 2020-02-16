// Copyright 2020 The go-zeromq Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package zyre provides reliable group messaging over local area networks.
//
// For more informations, see https://zeromq.org.
package zyre

import (
	"context"
	"fmt"
	"time"
)

// Zyre structure
type Zyre struct {
	requests chan *Cmd   // Requests from Zyre to Node
	replies  chan *Cmd   // Replies from Node to Zyre
	events   chan *Event // Receives incoming cluster events
	uuid     string      // Copy of node UUID string
	name     string      // Copy of node name
}

// NewZyre creates a new Zyre node. Note that until you start the
// node it is silent and invisible to other nodes on the network.
// The node name is provided to other nodes during discovery.
func NewZyre(ctx context.Context) *Zyre {
	zyre := &Zyre{
		events:   make(chan *Event, 1000),
		requests: make(chan *Cmd),
		replies:  make(chan *Cmd),
	}

	// Create backend node
	newNode(ctx, zyre.requests, zyre.replies, zyre.events)
	return zyre
}

// UUID returns our node UUID, after successful initialization
func (zyre *Zyre) UUID() string {
	zyre.requests <- &Cmd{ID: "UUID"}
	r := <-zyre.replies
	zyre.uuid = r.Payload.(string)
	return zyre.uuid
}

// Name returns our node name, after successful initialization. By default
// is taken from the UUID and shortened.
func (zyre *Zyre) Name() string {
	zyre.requests <- &Cmd{ID: "NAME"}
	r := <-zyre.replies
	zyre.name = r.Payload.(string)
	return zyre.name
}

// SetName sets node name.
func (zyre *Zyre) SetName(name string) *Zyre {
	zyre.requests <- &Cmd{ID: "SET NAME", Payload: name}
	return zyre
}

// SetHeader sets node header; these are provided to other nodes during
// discovery and come in each ENTER message.
func (zyre *Zyre) SetHeader(name string, format string, args ...interface{}) *Zyre {
	payload := fmt.Sprintf(format, args...)
	zyre.requests <- &Cmd{ID: "SET HEADER", Payload: []string{name, payload}}
	return zyre
}

// SetVerbose sets verbose mode; this tells the node to log all traffic as well
// as all major events.
func (zyre *Zyre) SetVerbose() *Zyre {
	zyre.requests <- &Cmd{ID: "SET VERBOSE", Payload: true}
	return zyre
}

// SetPort sets UDP beacon discovery port; defaults to 5670, this call overrides
// that so you can create independent clusters on the same network, for
// e.zyre. development vs. production. Has no effect after zyre.Start().
// FIXME make sure it has no effect after zyre.Start()
func (zyre *Zyre) SetPort(port int) *Zyre {
	zyre.requests <- &Cmd{ID: "SET PORT", Payload: port}
	return zyre
}

// SetInterval sets UDP beacon discovery interval. Default is instant
// beacon exploration followed by pinging every 1000 msecs.
func (zyre *Zyre) SetInterval(interval time.Duration) *Zyre {
	zyre.requests <- &Cmd{ID: "SET INTERVAL", Payload: interval}
	return zyre
}

// SetInterface sets network interface for UDP beacons. If you do not set this,
// CZMQ will choose an interface for you. On boxes with several interfaces you
// should specify which one you want to use, or strange things can happen.
func (zyre *Zyre) SetInterface(iface string) *Zyre {
	zyre.requests <- &Cmd{ID: "SET INTERFACE", Payload: iface}
	return zyre
}

// Start node, after setting header values. When you start a node it
// begins discovery and connection. Returns nil if OK, error if it wasn't
// possible to start the node.
func (zyre *Zyre) Start() error {
	zyre.requests <- &Cmd{ID: "START"}
	r := <-zyre.replies
	return r.Err
}

// Stop node; this signals to other peers that this node will go away.
// This is polite; however you can also just destroy the node without
// stopping it.
func (zyre *Zyre) Stop() {
	zyre.requests <- &Cmd{ID: "STOP"}
	<-zyre.replies
}

// Join a named group; after joining a group you can send messages to
// the group and all Zyre nodes in that group will receive them.
func (zyre *Zyre) Join(group string) {
	zyre.requests <- &Cmd{ID: "JOIN", Payload: group}
}

// Leave a group.
func (zyre *Zyre) Leave(group string) {
	zyre.requests <- &Cmd{ID: "LEAVE", Payload: group}
}

// Events returns a channel of events. The events may be a control
// event (ENTER, EXIT, JOIN, LEAVE) or data (WHISPER, SHOUT).
func (zyre *Zyre) Events() chan *Event {
	return zyre.events
}

// Whisper sends message to single peer, specified as a UUID string
func (zyre *Zyre) Whisper(peer string, payload []byte) {
	zyre.requests <- &Cmd{ID: "WHISPER", Payload: []string{peer, string(payload)}}
}

// Shout sends message to a named group.
func (zyre *Zyre) Shout(group string, payload []byte) {
	zyre.requests <- &Cmd{ID: "SHOUT", Payload: []string{group, string(payload)}}
}

// Whispers sends formatted string to a single peer specified as UUID string
func (zyre *Zyre) Whispers(peer string, format string, args ...interface{}) {
	payload := fmt.Sprintf(format, args...)
	zyre.requests <- &Cmd{ID: "WHISPER", Payload: []string{peer, payload}}
}

// Shouts sends formatted string to a named group
func (zyre *Zyre) Shouts(group string, format string, args ...interface{}) {
	payload := fmt.Sprintf(format, args...)
	zyre.requests <- &Cmd{ID: "SHOUT", Payload: []string{group, payload}}
}

// Peers returns a list of current peer IDs.
func (zyre *Zyre) Peers() []string {
	zyre.requests <- &Cmd{ID: "PEERS"}
	r := <-zyre.replies
	return r.Payload.([]string)
}
