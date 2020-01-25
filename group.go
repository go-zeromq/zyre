// Copyright 2020 The go-zeromq Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package zyre

// Group group known to this node
type Group struct {
	name  string           // Group name
	peers map[string]*Peer // Peers in group
}

// NewGroup creates new group
func NewGroup(name string) *Group {
	return &Group{
		name:  name,
		peers: make(map[string]*Peer),
	}
}

// Join adds peer to group, ignores duplicate joins
func (g *Group) Join(peer *Peer) {
	g.peers[peer.Identity] = peer
	peer.status++
}

// Leave removes peer from group
func (g *Group) Leave(peer *Peer) {
	delete(g.peers, peer.Identity)
	peer.status++
}

// Send message to all peers in group
func (g *Group) Send(msg *ZreMsg) {
	for _, peer := range g.peers {
		peer.Send(msg)
	}
}
