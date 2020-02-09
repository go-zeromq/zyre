// Copyright 2020 The go-zeromq Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package zyre

import (
	"context"
	"math/rand"
	"reflect"
	"testing"
	"time"
)

func randomPort() int {
	min := 8000
	max := 9000
	rand.Seed(time.Now().UnixNano())
	return rand.Intn(max-min) + min
}

func waitFor(cond func() bool, timeout time.Duration) {
	after := time.After(timeout)
	for {
		select {
		case <-after:
			return
		default:
			if cond() {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
}

func TestNodeRecvAPI(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	requests := make(chan *Cmd)
	replies := make(chan *Cmd, 1)
	events := make(chan *Event)
	n := newNode(ctx, requests, replies, events)

	if len(n.name) != 6 {
		t.Errorf("expected len(n.name)==6 but got %d (%s)", len(n.name), n.name)
	}

	// test SETs
	sets := []struct {
		cmd     string
		payload interface{}
		got     func() interface{}
	}{
		{cmd: "SET NAME", payload: "name", got: func() interface{} { return n.name }},
		{cmd: "SET HEADER", payload: []string{"key", "value"},
			got: func() interface{} { return []string{"key", "value"} }},
		{cmd: "SET VERBOSE", payload: true, got: func() interface{} { return n.verbose }},
		{cmd: "SET PORT", payload: 1111, got: func() interface{} { return n.beaconPort }},
		{cmd: "SET INTERVAL", payload: 1 * time.Second, got: func() interface{} { return n.interval }},
		{cmd: "SET INTERFACE", payload: "iface", got: func() interface{} { return n.beacon.ifc }},
	}
	for _, tt := range sets {
		t.Run(tt.cmd, func(t *testing.T) {
			n.recvAPI(&Cmd{ID: tt.cmd, Payload: tt.payload})
			if got := tt.got(); !reflect.DeepEqual(got, tt.payload) {
				t.Errorf("expected %v, got %v", tt.payload, got)
			}
		})
	}

	// test GETs
	n.uuid = []byte("uuid")
	n.name = "name"
	gets := []struct {
		cmd     string
		payload interface{}
		reply   Cmd
	}{
		{cmd: "UUID", reply: Cmd{Payload: n.uuid}},
		{cmd: "NAME", reply: Cmd{Payload: n.name}},
	}
	for _, tt := range gets {
		t.Run(tt.cmd, func(t *testing.T) {
			n.recvAPI(&Cmd{ID: tt.cmd, Payload: tt.payload})
			if got := <-replies; !reflect.DeepEqual(*got, tt.reply) {
				t.Errorf("expected %v, got %v", tt.reply, *got)
			}
		})
	}

}

func TestNodeStart(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// create nodes
	nNodes := 2
	requests := make([]chan *Cmd, nNodes)
	replies := make([]chan *Cmd, nNodes)
	events := make([]chan *Event, nNodes)
	var nodes = make([]*node, nNodes)
	for i := 0; i < nNodes; i++ {
		requests[i] = make(chan *Cmd)
		replies[i] = make(chan *Cmd)
		events[i] = make(chan *Event)
		nodes[i] = newNode(ctx, requests[i], replies[i], events[i])
		requests[i] <- &Cmd{ID: "SET INTERVAL", Payload: 10 * time.Millisecond}
	}

	peersIds := func(i int) []string {
		requests[i] <- &Cmd{ID: "PEERS"}
		r := <-replies[i]
		return r.Payload.([]string)
	}

	// check beacon is off
	time.Sleep(20 * time.Millisecond)
	if len(peersIds(0)) != 0 {
		t.Errorf("expected no peers, go %v", len(peersIds(0)))
	}

	// start nodes
	for i := 0; i < nNodes; i++ {
		requests[i] <- &Cmd{ID: "START"}
		<-replies[i]
	}

	// check peers
	waitFor(func() bool { return len(peersIds(0)) != 0 }, 20*time.Millisecond)
	if len(peersIds(0)) != 1 {
		t.Errorf("expected 1 peer, go %v", len(peersIds(0)))
	}

	// stop nodes
	for i := 0; i < nNodes; i++ {
		requests[i] <- &Cmd{ID: "STOP"}
		<-replies[i]
	}

	// check peers
	waitFor(func() bool { return len(nodes[0].peers) == 0 }, 20*time.Millisecond)
	if len(nodes[0].peers) != 0 {
		t.Errorf("expected 0 peer, go %v", len(nodes[0].peers))
	}
}
