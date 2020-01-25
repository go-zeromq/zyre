// Copyright 2020 The go-zeromq Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package zyre

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"

	zmq "github.com/go-zeromq/zmq4"
)

const (
	// Signature of the message
	Signature uint16 = 0xAAA0 | 1
	// Version number
	Version byte = 2
)

// Message IDs
const (
	HelloID   uint8 = 1
	WhisperID uint8 = 2
	ShoutID   uint8 = 3
	JoinID    uint8 = 4
	LeaveID   uint8 = 5
	PingID    uint8 = 6
	PingOkID  uint8 = 7
)

// ZreMsg work with ZRE messages
type ZreMsg struct {
	routingID []byte            //  Routing_id from ROUTER, if any
	id        uint8             //  ZreMsg message ID
	sequence  uint16            //  Cyclic sequence number
	Endpoint  string            //  Sender connect endpoint
	Groups    []string          //  List of groups sender is in
	Status    byte              //  Sender groups status value
	Name      string            //  Sender public name
	Headers   map[string]string //  Sender header properties
	Content   []byte            //  Wrapped message content
	Group     string            //  Name of Group
}

// NewZreMsg creates new ZreMsg message with messageID
func NewZreMsg(id uint8) *ZreMsg {
	msg := &ZreMsg{
		id:      id,
		Headers: make(map[string]string),
	}
	return msg
}

// Recv reads ZreMsg from ZMQ Socket
func Recv(socket zmq.Socket) (*ZreMsg, error) {
	input, err := socket.Recv()
	msg := NewZreMsg(0)
	if err != nil {
		return msg, err
	}
	frames := input.Frames
	if frames == nil { // FIXME: until pull request zmq4
		return msg, errors.New("empty message")
	}
	// If we're reading from a ROUTER socket, get routingID
	if socket.Type() == zmq.Router {
		msg.routingID = frames[0]
		frames = frames[1:]
	}
	var buffer *bytes.Buffer
	buffer = bytes.NewBuffer(frames[0])
	if signature := getNumber2(buffer); signature != Signature {
		return msg, fmt.Errorf("invalid signature %X != %X", Signature, signature)
	}
	msg.id = getNumber1(buffer)
	if version := getNumber1(buffer); version != Version {
		return msg, errors.New("malformed version message")
	}
	msg.sequence = getNumber2(buffer)

	switch msg.id {
	case HelloID:
		unpackHello(buffer, msg)
	case WhisperID:
		unpackContent(frames[1:], msg)
	case ShoutID:
		msg.Group = getString(buffer)
		unpackContent(frames[1:], msg)
	case JoinID, LeaveID:
		msg.Group = getString(buffer)
		msg.Status = getNumber1(buffer)
	}

	return msg, err
}

// Send sends ZreMsg through ZMQ socket.
func Send(socket zmq.Socket, msg *ZreMsg) error {
	var buffer bytes.Buffer
	putNumber2(&buffer, Signature)
	putNumber1(&buffer, msg.id)
	putNumber1(&buffer, Version)
	putNumber2(&buffer, msg.sequence)

	switch msg.id {
	case HelloID:
		packHello(&buffer, msg)
	case ShoutID:
		putString(&buffer, msg.Group)
	case JoinID, LeaveID:
		putString(&buffer, msg.Group)
		putNumber1(&buffer, msg.Status)
	}

	frames := [][]byte{buffer.Bytes()}
	// If we're sending to a ROUTER, we send the routingID first
	if socket.Type() == zmq.Router {
		frames = append([][]byte{msg.routingID}, frames...)
	}
	if msg.id == WhisperID || msg.id == ShoutID {
		frames = append(frames, msg.Content)
	}
	if socket.Type() == zmq.Router || msg.id == WhisperID ||
		msg.id == ShoutID {
		return socket.SendMulti(zmq.Msg{Frames: frames})
	}
	return socket.Send(zmq.Msg{Frames: frames})
}

func packHello(buffer *bytes.Buffer, msg *ZreMsg) {
	putString(buffer, msg.Endpoint)
	binary.Write(buffer, binary.BigEndian, uint32(len(msg.Groups)))
	for _, val := range msg.Groups {
		putLongString(buffer, val)
	}
	putNumber1(buffer, msg.Status)
	putString(buffer, msg.Name)
	binary.Write(buffer, binary.BigEndian, uint32(len(msg.Headers)))
	for key, val := range msg.Headers {
		putString(buffer, key)
		putLongString(buffer, val)
	}
}

func unpackHello(buffer *bytes.Buffer, msg *ZreMsg) {
	msg.Endpoint = getString(buffer)
	var groupsSize uint32
	binary.Read(buffer, binary.BigEndian, &groupsSize)
	for ; groupsSize != 0; groupsSize-- {
		msg.Groups = append(msg.Groups, getLongString(buffer))
	}
	msg.Status = getNumber1(buffer)
	msg.Name = getString(buffer)
	var headersSize uint32
	binary.Read(buffer, binary.BigEndian, &headersSize)
	for ; headersSize != 0; headersSize-- {
		key := getString(buffer)
		val := getLongString(buffer)
		msg.Headers[key] = val
	}
}

func unpackContent(frames [][]byte, msg *ZreMsg) {
	if 0 <= len(frames)-1 {
		msg.Content = frames[0]
	}
}

func putData(buffer *bytes.Buffer, data interface{}) {
	binary.Write(buffer, binary.BigEndian, data)
}

func putNumber1(buffer *bytes.Buffer, number byte) {
	putData(buffer, number)
}

func putNumber2(buffer *bytes.Buffer, number uint16) {
	putData(buffer, number)
}

func putString(buffer *bytes.Buffer, str string) {
	size := len(str)
	putData(buffer, byte(size))
	putData(buffer, []byte(str[0:size]))
}

func putLongString(buffer *bytes.Buffer, str string) {
	size := len(str)
	putData(buffer, uint32(size))
	putData(buffer, []byte(str[0:size]))
}

func getNumber1(buffer *bytes.Buffer) byte {
	var num byte
	binary.Read(buffer, binary.BigEndian, &num)
	return num
}

func getNumber2(buffer *bytes.Buffer) uint16 {
	var num uint16
	binary.Read(buffer, binary.BigEndian, &num)
	return num
}

func getString(buffer *bytes.Buffer) string {
	var size byte
	binary.Read(buffer, binary.BigEndian, &size)
	str := make([]byte, size)
	binary.Read(buffer, binary.BigEndian, &str)
	return string(str)
}

func getLongString(buffer *bytes.Buffer) string {
	var size uint32
	binary.Read(buffer, binary.BigEndian, &size)
	str := make([]byte, size)
	binary.Read(buffer, binary.BigEndian, &str)
	return string(str)
}
