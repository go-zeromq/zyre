// Copyright 2020 The go-zeromq Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package zyre

import (
	"bytes"
	"context"
	"testing"

	zmq "github.com/go-zeromq/zmq4"
)

func TestNumber(t *testing.T) {
	var buffer bytes.Buffer
	putNumber1(&buffer, Version)
	version := getNumber1(&buffer)
	if version != Version {
		t.Errorf("want %d, got %d", Version, version)
	}
	var givenSeq uint16 = 234
	putNumber2(&buffer, givenSeq)
	seq := getNumber2(&buffer)
	if seq != givenSeq {
		t.Errorf("want %d, got %d", givenSeq, seq)
	}
}

func TestString(t *testing.T) {
	var buffer bytes.Buffer
	givenStr := "hello"
	putString(&buffer, givenStr)
	str := getString(&buffer)
	if str != givenStr {
		t.Errorf("want %s, got %s", givenStr, str)
	}
	givenStr = "bye"
	putLongString(&buffer, givenStr)
	str = getLongString(&buffer)
	if str != givenStr {
		t.Errorf("want %s, got %s", givenStr, str)
	}
}

func TestZreMsg(t *testing.T) {
	tests := []struct {
		name string
		id   uint8
	}{
		{name: "Hello", id: HelloID},
		{name: "Whisper", id: WhisperID},
		{name: "Shout", id: ShoutID},
		{name: "Join", id: JoinID},
		{name: "Leave", id: LeaveID},
		{name: "Ping", id: PingID},
		{name: "PingOK", id: PingOkID},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			endpoint := "inproc://selftest-" + tt.name
			routingID := "Test"
			dealer := zmq.NewDealer(ctx, zmq.WithID(zmq.SocketIdentity(routingID)))
			defer dealer.Close()
			checkError(dealer.Listen(endpoint), t)

			router := zmq.NewRouter(ctx, zmq.WithID(zmq.SocketIdentity("router")))
			defer router.Close()
			checkError(router.Dial(endpoint), t)

			// Create a ZreMsg message and send it through the wire
			expectedMsg := NewZreMsg(tt.id)
			expectedMsg.routingID = []byte(routingID)
			expectedMsg.sequence = 123
			expectedMsg.Endpoint = "Endpoint"
			expectedMsg.Groups = []string{"Group1: A", "Group2: B"}
			expectedMsg.Status = 123
			expectedMsg.Name = "Name"
			expectedMsg.Headers = map[string]string{"Header1": "1", "Header2": "2"}
			expectedMsg.Group = "Group"
			expectedMsg.Content = []byte("Some Content")

			// dealer sends expectedMsg to router
			checkError(Send(dealer, expectedMsg), t)
			msg, err := Recv(router)
			checkError(err, t)
			checkMsg(msg, *expectedMsg, t)

			// router sends back received msg to dealer
			checkError(Send(router, msg), t)
			msg, err = Recv(dealer)
			checkError(err, t)
			expectedMsg.routingID = []byte{}
			checkMsg(msg, *expectedMsg, t)
		})
	}
}

func TestRecvError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	endpoint := "inproc://selftest-error"
	routingID := "Test"
	dealer := zmq.NewDealer(ctx, zmq.WithID(zmq.SocketIdentity(routingID)))
	defer dealer.Close()
	checkError(dealer.Listen(endpoint), t)

	router := zmq.NewRouter(ctx, zmq.WithID(zmq.SocketIdentity("router")))
	defer router.Close()
	checkError(router.Dial(endpoint), t)

	// dealer sends Msg with no signature
	dealer.Send(zmq.NewMsgString(""))
	_, err := Recv(router)
	if err == nil {
		t.Error("error expected")
	}
	// dealer sends Msg with incorrect version
	var buffer bytes.Buffer
	putNumber2(&buffer, Signature)
	putNumber1(&buffer, HelloID)
	putNumber1(&buffer, 1)
	dealer.Send(zmq.NewMsgFrom(buffer.Bytes()))
	_, err = Recv(router)
	if err == nil {
		t.Error("error expected")
	}

	// socket Recv returns an error
	cancel()
	_, err = Recv(router)
	if err == nil {
		t.Error("error expected")
	}
}

func checkMsg(msg *ZreMsg, expectedMsg ZreMsg, t *testing.T) {
	switch msg.id {
	case HelloID:
		expectedMsg.id = HelloID
		expectedMsg.Group = ""
		expectedMsg.Content = []byte{}
		checkZreMsg(msg, &expectedMsg, t)
	case PingID, PingOkID:
		expected := ZreMsg{routingID: expectedMsg.routingID, id: msg.id,
			sequence: expectedMsg.sequence}
		checkZreMsg(msg, &expected, t)
	case JoinID, LeaveID:
		expected := ZreMsg{routingID: expectedMsg.routingID, id: msg.id,
			sequence: expectedMsg.sequence, Group: expectedMsg.Group,
			Status: expectedMsg.Status}
		checkZreMsg(msg, &expected, t)
	case ShoutID:
		expected := ZreMsg{routingID: expectedMsg.routingID, id: msg.id,
			sequence: expectedMsg.sequence, Group: expectedMsg.Group,
			Content: expectedMsg.Content}
		checkZreMsg(msg, &expected, t)
	case WhisperID:
		expected := ZreMsg{routingID: expectedMsg.routingID, id: msg.id,
			sequence: expectedMsg.sequence, Content: expectedMsg.Content}
		checkZreMsg(msg, &expected, t)
	}
}

func checkError(err error, t *testing.T) {
	if err != nil {
		t.Error(err)
	}
}

func checkZreMsg(msg *ZreMsg, expected *ZreMsg, t *testing.T) {
	if string(msg.routingID) != string(expected.routingID) {
		t.Errorf("want %s, got %s", expected.routingID, msg.routingID)
	}
	if msg.id != expected.id {
		t.Errorf("want %d, got %d", expected.id, msg.id)
	}
	if msg.sequence != expected.sequence {
		t.Errorf("want %d, got %d", expected.sequence, msg.sequence)
	}
	if msg.Endpoint != expected.Endpoint {
		t.Errorf("want %s, got %s", expected.Endpoint, msg.Endpoint)
	}
	for idx, str := range expected.Groups {
		if msg.Groups[idx] != str {
			t.Errorf("want %s, got %s", str, msg.Groups[idx])
		}
	}
	if msg.Status != expected.Status {
		t.Errorf("want %d, got %d", expected.Status, msg.Status)
	}
	if msg.Name != expected.Name {
		t.Errorf("want %s, got %s", expected.Name, msg.Name)
	}
	if msg.Group != expected.Group {
		t.Errorf("want %s, got %s", expected.Group, msg.Group)
	}
	if string(msg.Content) != string(expected.Content) {
		t.Errorf("want %s, got %s", expected.Content, msg.Content)
	}
	for key, val := range expected.Headers {
		if msg.Headers[key] != val {
			t.Errorf("want %s, got %s", val, msg.Headers[key])
		}
	}
}
