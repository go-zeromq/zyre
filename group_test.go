// Copyright 2020 The go-zeromq Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package zyre

import (
	"strconv"
	"testing"
)

type mockPeerMailboxSend struct {
	peerMailboxSocket
	nSent int
}

func (p *mockPeerMailboxSend) send(*ZreMsg) error {
	p.nSent++
	return nil
}

func TestGroup(t *testing.T) {
	group := NewGroup("group")
	if len(group.peers) != 0 {
		t.Errorf("got %v peers, want 0", len(group.peers))
	}

	// make some peers and join group
	mailbox := &mockPeerMailboxSend{}
	nPeers := 2
	peers := make([]*Peer, nPeers)
	for i := 0; i < nPeers; i++ {
		id := "peer" + strconv.Itoa(i)
		peers[i] = &Peer{Identity: id, Name: id, connected: true, mailbox: mailbox}
		group.Join(peers[i])
	}

	if len(group.peers) != nPeers {
		t.Errorf("got %v peers, want %v", len(group.peers), nPeers)
	}

	// send msg to group
	group.Send(&ZreMsg{})
	if mailbox.nSent != nPeers {
		t.Errorf("got %v msgs, want %v", mailbox.nSent, nPeers)
	}

	// leave group
	for _, p := range peers {
		group.Leave(p)
	}
}
