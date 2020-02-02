// Copyright 2020 The go-zeromq Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package zyre

import (
	"bytes"
	"log"
	"net"
	"os"
	"sync"
	"time"
)

const (
	beaconMax = 255
)

// Cmd type to transfer commands through channels
type Cmd struct {
	ID      string
	Payload interface{}
	Err     error
}

// Signal contains the body of the beacon (Transmit) and the source address
type Signal struct {
	Addr     string
	Transmit []byte
}

// Beacon defines main structure of the application
type Beacon struct {
	Signals    chan *Signal
	Cmds       chan *Cmd
	Replies    chan *Cmd
	msgs       chan *Signal
	terminated chan interface{} // API shut us down

	conn     Connection
	port     int           // UDP port number we work on
	interval time.Duration // Beacon broadcast interval
	transmit []byte        // Beacon transmit data
	filter   []byte        // Beacon filter data

	verbose bool
	ifc     string
	wg      sync.WaitGroup
}

// NewBeacon creates a new beacon on a certain UDP port.
func NewBeacon() *Beacon {
	b := &Beacon{
		Signals:    make(chan *Signal, 50),
		Cmds:       make(chan *Cmd),
		Replies:    make(chan *Cmd),
		interval:   1 * time.Second,
		msgs:       make(chan *Signal),
		terminated: make(chan interface{}),
		ifc:        os.Getenv("ZSYS_INTERFACE"),
	}
	b.wg.Add(1)
	go b.run()
	return b
}

func (b *Beacon) configure() error {
	var err error
	b.conn, err = NewConn(b.port)
	if err != nil {
		return err
	}

	var ifs []net.Interface
	if b.ifc != "" {
		ifc, err := net.InterfaceByName(b.ifc)
		if err != nil {
			return err
		}
		ifs = append(ifs, *ifc)
	} else {
		ifs, err = net.Interfaces()
		if err != nil {
			return err
		}
	}

	for _, ifc := range ifs {
		err := b.conn.JoinGroup(&ifc)
		if err != nil {
			return err
		}
		break
	}
	return err
}

// Close terminates the beacon.
func (b *Beacon) Close() {
	b.SetBeaconCmd(&Cmd{ID: "$TERM"})
	b.wg.Wait()
}

// SetBeaconCmd ...
func (b *Beacon) SetBeaconCmd(req *Cmd) *Cmd {
	b.Cmds <- req
	rep := <-b.Replies
	return rep
}

func (b *Beacon) startListening() {
	b.wg.Add(1)
	go func() {
		defer b.wg.Done()

		for {
			select {
			case <-b.terminated:
				close(b.msgs)
				return
			default:
				buff := make([]byte, beaconMax)
				n, addr, err := b.conn.ReadFrom(buff)
				if err != nil {
					return
				}
				if len(addr) != 0 {
					b.msgs <- &Signal{addr.String(), buff[:n]}
				}
			}
		}
	}()
}

func (b *Beacon) handleCommand(r *Cmd, tick *<-chan time.Time) *Cmd {
	if b.verbose {
		log.Printf("beacon: API command=%s", r.ID)
	}
	rep := &Cmd{}

	switch r.ID {
	case "VERBOSE":
		b.verbose = true
	case "CONFIGURE":
		b.port = r.Payload.(int)
		err := b.configure()
		rep = &Cmd{Payload: b.conn.Addr(), Err: err}
	case "PUBLISH":
		b.transmit = r.Payload.([]byte)
	case "SILENCE":
		b.transmit = nil
	case "SUBSCRIBE":
		b.filter = r.Payload.([]byte)
		b.startListening()
	case "UNSUBSCRIBE":
		b.filter = nil
	case "INTERVAL":
		b.interval = r.Payload.(time.Duration)
		*tick = time.Tick(b.interval)
	case "INTERFACE":
		b.ifc = r.Payload.(string)
	case "$TERM":
		close(b.terminated)
	default:
		if b.verbose {
			log.Printf("beacon: invalid command: %s", r.ID)
		}
	}
	return rep
}

func (b *Beacon) run() {
	defer b.wg.Done()
	tick := time.Tick(b.interval)

	for {
		select {
		case r := <-b.Cmds:
			b.Replies <- b.handleCommand(r, &tick)
		case s, ok := <-b.msgs:
			if !ok {
				return
			}
			// send only if subscribed to received msg and is not from me
			if bytes.HasPrefix(s.Transmit, b.filter) &&
				!bytes.Equal(s.Transmit, b.transmit) {
				b.Signals <- &Signal{s.Addr, s.Transmit}
			}
		case <-tick:
			if b.transmit != nil {
				err := b.conn.WriteTo(b.transmit)
				if err != nil && b.verbose {
					log.Printf("beacon: ticker failed %s\n", err)
				}
			}
		}
	}
}
