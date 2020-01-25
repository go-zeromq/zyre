// Copyright 2020 The go-zeromq Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package zyre

import (
	"context"
	"testing"

	zmq "github.com/go-zeromq/zmq4"
)

type mockPeerMailbox struct {
	sequence uint16
}

func (p *mockPeerMailbox) connect(identity []byte, endpoint string) error {
	return nil
}

func (p *mockPeerMailbox) close() {
}

func (p *mockPeerMailbox) send(m *ZreMsg) error {
	p.sequence = m.sequence
	return nil
}

func TestPeer(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p := NewPeer(ctx, "peer1")
	mailbox := &mockPeerMailbox{}
	p.mailbox = mailbox
	p.Connect([]byte("from"), "endpoint")
	if !p.connected {
		t.Error("peer not connected")
	}
	if p.ready {
		t.Error("peer should not be ready")
	}
	_, ok := p.Header("no header")
	if ok {
		t.Error("want header not ok")
	}
	m := NewZreMsg(HelloID)
	p.Send(m)
	if mailbox.sequence != 1 {
		t.Errorf("want sequence %v, got %v", 1, mailbox.sequence)
	}
	p.Disconnect()
	if p.connected {
		t.Error("peer still connected")
	}
	p.Destroy()
}

func TestPeerNetwork(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mailbox := zmq.NewDealer(ctx)
	defer mailbox.Close()

	err := mailbox.Listen("tcp://127.0.0.1:5551")
	if err != nil {
		t.Fatal(err)
	}

	peer := NewPeer(ctx, "you")
	err = peer.Connect([]byte("me"), "tcp://127.0.0.1:5551")
	if err != nil {
		t.Fatal(err)
	}
	if !peer.connected {
		t.Fatal("Peer should be connected")
	}

	m := NewZreMsg(HelloID)
	m.Endpoint = "tcp://127.0.0.1:5551"
	peer.Send(m)

	// FIXME
	// exp, err := m.Marshal()
	// if err != nil {
	// 	t.Fatal(err)
	// }

	// msg, err := mailbox.Recv()
	// if err != nil {
	// 	t.Fatal(err)
	// }

	// if !bytes.Equal(msg.Bytes(), exp) {
	// 	t.Error("Hello message was corrupted")
	// }

	peer.Destroy()
}
