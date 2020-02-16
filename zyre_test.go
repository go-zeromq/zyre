// Copyright 2020 The go-zeromq Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package zyre

import (
	"bytes"
	"context"
	"log"
	"strconv"
	"testing"
	"time"
)

func makeZyres(ctx context.Context, n, port int, group string) []*Zyre {
	nodes := make([]*Zyre, n)
	var err error
	for i := range nodes {
		nodes[i] = NewZyre(ctx)
		if err != nil {
			log.Fatal(err)
		}
		nodes[i].
			SetPort(port).
			SetName("node" + strconv.Itoa(i)).
			SetInterval(10 * time.Millisecond).
			Start()
		nodes[i].Join(group)
	}
	return nodes
}

func waitForEvent(t *testing.T, eventType string, events <-chan *Event) *Event {
	event := &Event{}
	after := time.After(2 * time.Second)
	select {
	case event = <-events:
		if event.Type != eventType {
			t.Errorf("want event %q, got %v", eventType, event.Type)
		}
	case <-after:
		t.Errorf("want event %q, none received", eventType)
	}
	return event
}

func TestZyre(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// create nodes
	port := randomPort()
	group := "GLOBAL-" + strconv.Itoa(port)
	nodes := makeZyres(ctx, 2, port, group)
	for i := 0; i < len(nodes); i++ {
		want := "node" + strconv.Itoa(i)
		if nodes[i].Name() != want {
			t.Errorf("want name %q, got %q", want, nodes[i].Name())
		}
	}

	// wait till connection
	waitFor(func() bool { return len(nodes[0].Peers()) != 0 }, 1*time.Second)
	peers := nodes[0].Peers()
	if len(peers) != 1 {
		t.Errorf("want 1 peer, got %v", len(peers))
	}

	// recv ENTER event
	event := waitForEvent(t, "ENTER", nodes[1].Events())
	if event.PeerName != "node0" {
		t.Errorf("want node0, got %s", event.PeerName)
	}

	// recv JOIN event
	event = waitForEvent(t, "JOIN", nodes[1].Events())

	// SHOUT
	want := "Hi there"
	nodes[0].Shout(group, []byte(want))
	event = waitForEvent(t, "SHOUT", nodes[1].Events())
	if !bytes.Equal(event.Msg, []byte(want)) {
		t.Errorf("want %q, got %q", want, string(event.Msg))
	}

	// WHISPER
	nodes[0].Whisper(peers[0], []byte(want))
	event = waitForEvent(t, "WHISPER", nodes[1].Events())
	if !bytes.Equal(event.Msg, []byte(want)) {
		t.Errorf("want %q, got %q", want, string(event.Msg))
	}

	// LEAVE
	nodes[0].Leave(group)
	event = waitForEvent(t, "LEAVE", nodes[1].Events())

	// STOP
	nodes[0].Stop()

	waitFor(func() bool { return len(nodes[1].Peers()) == 0 }, 1*time.Second)
	peers = nodes[1].Peers()
	if len(peers) != 0 {
		t.Errorf("want 0 peer, got %v", len(peers))
	}
}
