// Copyright 2020 The go-zeromq Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package zyre

import (
	"context"
	"fmt"
	"time"

	zmq "github.com/go-zeromq/zmq4"
)

var (
	peerExpired = 5 * time.Second // expire after...
	peerEvasive = 3 * time.Second // mark evasive after..
)

// peerMailbox interface to mock networking
type peerMailbox interface {
	connect(identity []byte, endpoint string) error
	close()
	send(msg *ZreMsg) error
}

// Peer one of our peers in a ZRE network
type Peer struct {
	Identity     string                          // Identity UUID
	Name         string                          // Peer's public name
	Headers      map[string]string               // Peer headers
	mailbox      peerMailbox                     // Peer connection interface
	endpoint     string                          // Endpoint connected to
	origin       string                          // Origin node's public name
	evasiveAt    time.Time                       // Peer is being evasive
	expiredAt    time.Time                       // Peer has expired by now
	connected    bool                            // Peer will send messages
	ready        bool                            // Peer has said Hello to us
	status       byte                            // Our status counter
	sentSequence uint16                          // Outgoing message sequence
	wantSequence uint16                          // Incoming message sequence
	sendMethod   func(zmq.Socket, *ZreMsg) error // send method to be used
}

// NewPeer creates new peer
func NewPeer(ctx context.Context, identity string) (p *Peer) {
	p = &Peer{
		Identity: identity,
		Name:     fmt.Sprintf("%.6s", identity),
		Headers:  make(map[string]string),
		mailbox:  &peerMailboxSocket{ctx: ctx},
	}
	p.Refresh()
	return
}

// Destroy and disconnect peer
func (p *Peer) Destroy() {
	p.Disconnect()
	for k := range p.Headers {
		delete(p.Headers, k)
	}
}

// Connect peer mailbox
// Configures mailbox and connects to peer's router endpoint
func (p *Peer) Connect(from []byte, endpoint string) (err error) {
	// Set our own identity on the socket so that receiving node
	// knows who each message came from. Note that we cannot use
	// the UUID directly as the identity since it may contain a
	// zero byte at the start, which libzmq does not like for
	// historical and arguably bogus reasons that it nonetheless
	// enforces.
	routingID := append([]byte{1}, from...)
	p.mailbox.connect(routingID, endpoint)
	p.endpoint = endpoint
	p.connected = true
	p.ready = false
	return nil
}

// Disconnect peer mailbox
// No more messages will be sent to peer until connected again
func (p *Peer) Disconnect() {
	if p.connected {
		p.mailbox.close()
		p.endpoint = ""
		p.connected = false
		p.ready = false
	}
}

// Send message to peer
func (p *Peer) Send(msg *ZreMsg) error {
	var err error
	if p.connected {
		p.sentSequence++
		msg.sequence = p.sentSequence
		err = p.mailbox.send(msg)
		if err != nil {
			p.Disconnect()
		}
	}
	return err
}

// Refresh registers activity at peer
func (p *Peer) Refresh() {
	p.evasiveAt = time.Now().Add(peerEvasive)
	p.expiredAt = time.Now().Add(peerExpired)
}

// Header gets peer header value
func (p *Peer) Header(key string) (string, bool) {
	v, ok := p.Headers[key]
	return v, ok
}

// MessagesLost check if messages were lost from peer, returns true if they were
// FIXME not same implementation as zyre or pyre
func (p *Peer) MessagesLost(msg *ZreMsg) bool {
	p.wantSequence++
	valid := p.wantSequence == msg.sequence
	if !valid {
		p.wantSequence--
	}
	return valid
}

// SetExpired sets expired.
func SetExpired(expired time.Duration) {
	peerExpired = expired
}

// SetEvasive sets evasive.
func SetEvasive(evasive time.Duration) {
	peerEvasive = evasive
}

// peerMailboxSocket connects to ZMQ socket
type peerMailboxSocket struct {
	ctx     context.Context // ZMQ context
	mailbox zmq.Socket      // Socket through to peer
}

// connect creates dealer and connects to endpoint
func (p *peerMailboxSocket) connect(identity []byte, endpoint string) error {
	// Create new outgoing socket (drop any messages in transit)
	p.mailbox = zmq.NewDealer(p.ctx, zmq.WithID(zmq.SocketIdentity(identity)))
	// Set a high-water mark that allows for reasonable activity
	p.mailbox.SetOption(zmq.OptionHWM, int(peerExpired*time.Microsecond))
	// FIXME missing in zmq4
	// Send messages immediately or return EAGAIN
	// zsock_set_sndtimeo (self->mailbox, 0);
	// Connect through to peer node
	err := p.mailbox.Dial(endpoint)
	return err
}

// closes the mailbox connection
func (p *peerMailboxSocket) close() {
	p.mailbox.Close()
	p.mailbox = nil
}

// sends msg through the mailbox
func (p *peerMailboxSocket) send(msg *ZreMsg) error {
	return Send(p.mailbox, msg)
}
