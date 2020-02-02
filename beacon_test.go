// Copyright 2020 The go-zeromq Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package zyre

import (
	"bytes"
	"math/rand"
	"reflect"
	"testing"
	"time"
)

func TestAPICommand(t *testing.T) {
	b := &Beacon{}
	reqs := []struct {
		id      string
		payload interface{}
		got     func() interface{}
	}{
		{id: "CONFIGURE", payload: 5555,
			got: func() interface{} { return b.port }},
		{id: "PUBLISH", payload: []byte("bla"),
			got: func() interface{} { return b.transmit }},
		{id: "SILENCE", payload: []byte(nil),
			got: func() interface{} { return b.transmit }},
		{id: "UNSUBSCRIBE", payload: []byte(nil),
			got: func() interface{} { return b.filter }},
		{id: "INTERVAL", payload: 1 * time.Second,
			got: func() interface{} { return b.interval }},
		{id: "INTERFACE", payload: "eth",
			got: func() interface{} { return b.ifc }},
		{id: "SUBSCRIBE", payload: []byte("bar"),
			got: func() interface{} { return b.filter }},
		{id: "VERBOSE", payload: true,
			got: func() interface{} { return b.verbose }},
	}
	for _, tt := range reqs {
		tick := time.Tick(1 * time.Second)
		t.Run(tt.id, func(t *testing.T) {
			b.handleCommand(&Cmd{ID: tt.id, Payload: tt.payload}, &tick)
			if got := tt.got(); !reflect.DeepEqual(got, tt.payload) {
				t.Errorf("expected %v, got %v", tt.payload, got)
			}
		})
		b.handleCommand(&Cmd{ID: "INVALID"}, &tick)
	}
}

func TestBeacon(t *testing.T) {
	port := random(5670, 15670)
	beacon1 := makeBeacon(port, "BEACON1", "BEACON", t)
	defer beacon1.Close()
	beacon2 := makeBeacon(port, "BEACON2", "BEACON1", t)
	defer beacon2.Close()
	beacon3 := makeBeacon(port, "FOO", "BAR", t)
	defer beacon3.Close()

	for i := 0; i < 5; i++ {
		select {
		case signal := <-beacon1.Signals:
			expected := []byte("BEACON2")
			if !bytes.Equal(expected, signal.Transmit) {
				t.Errorf("expected %s, got %s", expected, signal.Transmit)
			}
		case signal := <-beacon2.Signals:
			expected := []byte("BEACON1")
			if !bytes.Equal(expected, signal.Transmit) {
				t.Errorf("expected %s, got %s", expected, signal.Transmit)
			}
		case signal := <-beacon3.Signals:
			t.Errorf("beacon3 shouldn't receive anything, got %q, %q", signal.Addr,
				signal.Transmit)
		}
	}
}

func makeBeacon(port int, publish string, subscribe string, t *testing.T) *Beacon {
	b := NewBeacon()
	b.SetBeaconCmd(&Cmd{ID: "CONFIGURE", Payload: port})
	b.SetBeaconCmd(&Cmd{ID: "INTERVAL", Payload: 1 * time.Millisecond})
	b.SetBeaconCmd(&Cmd{ID: "SUBSCRIBE", Payload: []byte(subscribe)})
	b.SetBeaconCmd(&Cmd{ID: "PUBLISH", Payload: []byte(publish)})
	return b
}

func random(min, max int) int {
	rand.Seed(time.Now().Unix())
	return rand.Intn(max-min) + min
}
