// Copyright 2020 The go-zeromq Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package zyre

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"strconv"
	"sync"
	"time"

	zmq "github.com/go-zeromq/zmq4"
)

type node struct {
	ctx        context.Context   // socket context
	requests   chan *Cmd         // requests from API
	replies    chan *Cmd         // replies to API
	events     chan *Event       // events to API
	msgs       chan *ZreMsg      // msgs from the inbox socket
	terminated chan interface{}  // API shut us down
	wg         sync.WaitGroup    // used to wait for all goroutines
	verbose    bool              // Log all traffic
	beaconPort int               // Beacon UDP port number
	interval   time.Duration     // Beacon interval
	beacon     *Beacon           // Beacon object
	uuid       []byte            // Our UUID
	inbox      zmq.Socket        // Our inbox socket (ROUTER)
	name       string            // Our public name
	endpoint   string            // Our public endpoint
	port       uint16            // Our inbox port, if any
	status     byte              // Our own change counter
	peers      map[string]*Peer  // Hash of known peers, fast lookup
	peerGroups map[string]*Group // Groups that our peers are in
	ownGroups  map[string]*Group // Groups that we are in
	headers    map[string]string // Our header values
}

// BeaconMsg frame has this format:
//
// Z R E       3 bytes
// version     1 byte 0x01 | 0x03
// UUID        16 bytes
// port        2 bytes in network order
type BeaconMsg struct {
	Protocol [3]byte
	Version  byte
	UUID     []byte
	Port     uint16
}

// pack converts Beacon to an array of bytes
func (b *BeaconMsg) pack() []byte {
	result := new(bytes.Buffer)
	binary.Write(result, binary.BigEndian, b.Protocol)
	binary.Write(result, binary.BigEndian, b.Version)
	binary.Write(result, binary.BigEndian, b.UUID)
	binary.Write(result, binary.BigEndian, b.Port)
	return result.Bytes()
}

// unpack converts Beacon to an array of bytes
func (b *BeaconMsg) unpack(msg []byte) {
	buffer := bytes.NewBuffer(msg)
	binary.Read(buffer, binary.BigEndian, &b.Protocol)
	binary.Read(buffer, binary.BigEndian, &b.Version)
	uuid := make([]byte, 16)
	binary.Read(buffer, binary.BigEndian, uuid)
	b.UUID = append(b.UUID, uuid...)
	binary.Read(buffer, binary.BigEndian, &b.Port)
}

const (
	beaconVersion    = 0x1
	zreDiscoveryPort = 5670
	reapInterval     = 1 * time.Second
)

// newNode creates a new node.
func newNode(ctx context.Context, requests chan *Cmd, replies chan *Cmd,
	events chan *Event) *node {
	n := &node{
		ctx:        ctx,
		requests:   requests,
		replies:    replies,
		events:     events,
		msgs:       make(chan *ZreMsg),
		terminated: make(chan interface{}),
		beaconPort: zreDiscoveryPort,
		beacon:     NewBeacon(),
		inbox:      zmq.NewRouter(ctx),
		peers:      make(map[string]*Peer),
		peerGroups: make(map[string]*Group),
		ownGroups:  make(map[string]*Group),
		headers:    make(map[string]string),
	}
	n.uuid = make([]byte, 16)
	io.ReadFull(rand.Reader, n.uuid)
	// Default name for node is first 6 characters of UUID:
	// the shorter string is more readable in logs
	n.name = fmt.Sprintf("%.3X", n.uuid)
	n.wg.Add(1)
	go n.actor()
	return n
}

func getEndpoint(addr string, port uint16) string {
	ip := net.ParseIP(addr)
	ipAddr := ip.String()
	if ip.To4() == nil {
		ipAddr = "[" + ipAddr + "]"
	}
	endpoint := fmt.Sprintf("tcp://%s:%d", ipAddr, port)
	return endpoint
}

func getHexUpper(data []byte) string {
	return fmt.Sprintf("%X", data)
}

// start node, return 0 if OK, 1 if not possible
func (n *node) start() error {
	// If application didn't bind explicitly, we grab an ephemeral port
	// on all available network interfaces. This is orthogonal to
	// beaconing, since we can connect to other peers and they will
	// gossip our endpoint to others.
	if n.beaconPort > 0 {
		// Start UDP beaconing
		addr := n.beacon.SetBeaconCmd(&Cmd{ID: "CONFIGURE", Payload: n.beaconPort})
		n.beacon.SetBeaconCmd(&Cmd{ID: "SUBSCRIBE", Payload: []byte("ZRE")})
		if n.interval > 0 {
			n.beacon.SetBeaconCmd(&Cmd{ID: "INTERVAL", Payload: n.interval})
		}
		err := n.inbox.Listen("tcp://*:0")
		if err != nil {
			return err
		}
		_, pstring, _ := net.SplitHostPort(n.inbox.Addr().String())
		port, _ := strconv.ParseUint(pstring, 0, 16)
		n.port = uint16(port)

		b := &BeaconMsg{
			Protocol: [...]byte{'Z', 'R', 'E'},
			Version:  beaconVersion,
			UUID:     n.uuid,
			Port:     n.port}
		n.beacon.SetBeaconCmd(&Cmd{ID: "PUBLISH", Payload: b.pack()})
		n.endpoint = getEndpoint(addr.Payload.(string), n.port)

		n.wg.Add(1)
		go func() {
			defer n.wg.Done()
			for {
				select {
				case <-n.terminated:
					close(n.msgs)
					return
				default:
					m, err := Recv(n.inbox)
					if err == nil {
						n.msgs <- m
					}
				}
			}
		}()
	}

	return nil
}

// stop node discovery and interconnection
func (n *node) stop() {
	if n.beacon != nil {
		b := &BeaconMsg{
			Protocol: [...]byte{'Z', 'R', 'E'},
			Version:  beaconVersion,
			UUID:     n.uuid,
			Port:     0}
		n.beacon.SetBeaconCmd(&Cmd{ID: "PUBLISH", Payload: b.pack()})
		time.Sleep(n.interval + 1*time.Millisecond) // FIXME
		n.beacon.Close()
	}
	n.beaconPort = 0
	n.inbox.Close()
}

// recvAPI handle the different control messages from the front-end
func (n *node) recvAPI(c *Cmd) {
	switch c.ID {
	case "UUID":
		n.replies <- &Cmd{Payload: n.uuid}
	case "NAME":
		n.replies <- &Cmd{Payload: n.name}
	case "SET NAME":
		n.name = c.Payload.(string)
	case "SET HEADER":
		keyPayload := c.Payload.([]string)
		n.headers[keyPayload[0]] = keyPayload[1]
	case "SET VERBOSE":
		n.verbose = c.Payload.(bool)
	case "SET PORT":
		n.beaconPort = c.Payload.(int)
	case "SET INTERVAL":
		n.interval = c.Payload.(time.Duration)
	case "SET INTERFACE":
		n.beacon.SetBeaconCmd(&Cmd{ID: "INTERFACE", Payload: c.Payload.(string)})
	case "START":
		err := n.start()
		n.replies <- &Cmd{Err: err}
	case "STOP":
		n.stop()
		close(n.terminated)
		go func() {
			n.wg.Wait()
			n.replies <- &Cmd{}
		}()
	case "WHISPER":
		peerPayload := c.Payload.([]string)
		// Get peer to send message to
		peer, ok := n.peers[peerPayload[0]]
		// Send frame on out to peer's mailbox, drop message
		// if peer doesn't exist (may have been destroyed)
		if ok {
			m := NewZreMsg(WhisperID)
			m.Content = []byte(peerPayload[1])
			peer.Send(m)
		}
	case "SHOUT":
		groupPayload := c.Payload.([]string)
		// Get group to send message to
		group := groupPayload[0]
		m := NewZreMsg(ShoutID)
		m.Group = group
		m.Content = []byte(groupPayload[1])
		if g, ok := n.peerGroups[group]; ok {
			g.Send(m)
		}
	case "JOIN":
		group := c.Payload.(string)
		if _, ok := n.ownGroups[group]; !ok {
			// Only send if we're not already in group
			n.ownGroups[group] = NewGroup(group)
			m := NewZreMsg(JoinID)
			m.Group = group
			n.status++
			m.Status = n.status
			for _, peer := range n.peers {
				peer.Send(m)
			}
		}
	case "LEAVE":
		group := c.Payload.(string)
		if _, ok := n.ownGroups[group]; ok {
			// Only send if we're actually in group
			// FIMXE function for JOIN and LEAVE
			m := NewZreMsg(LeaveID)
			m.Group = group
			n.status++
			m.Status = n.status
			for _, peer := range n.peers {
				peer.Send(m)
			}
			delete(n.ownGroups, group)
		}
	case "PEERS":
		var peersIDs []string
		for k := range n.peers {
			peersIDs = append(peersIDs, k)
		}
		n.replies <- &Cmd{Payload: peersIDs}
	default:
		log.Printf("invalid command %q", c.ID)
	}
}

// purgePeer deletes peer for a given endpoint
func purgePeer(peer *Peer, endpoint string) {
	if peer.endpoint == endpoint {
		peer.Disconnect()
	}
}

// requirePeer find or create peer via its UUID
func (n *node) requirePeer(identity string, endpoint string) (*Peer, error) {
	peer, ok := n.peers[identity]
	if !ok {
		// Purge any previous peer on same endpoint
		for _, p := range n.peers {
			purgePeer(p, endpoint)
		}

		peer = NewPeer(n.ctx, identity)
		n.peers[identity] = peer
		peer.origin = n.name
		err := peer.Connect(n.uuid, endpoint)
		if err != nil {
			return nil, err
		}

		// Handshake discovery by sending HELLO as first message
		m := NewZreMsg(HelloID)
		m.Endpoint = n.endpoint
		m.Status = n.status
		m.Name = n.name
		for key := range n.ownGroups {
			m.Groups = append(m.Groups, key)
		}
		for key, header := range n.headers {
			m.Headers[key] = header
		}
		peer.Send(m)
	}
	return peer, nil
}

// deletePeer removes peer from group, if it's a member
func deletePeer(peer *Peer, group *Group) {
	group.Leave(peer)
}

// removePeer removes a peer from our data structures
func (n *node) removePeer(peer *Peer) {
	if peer == nil {
		return
	}
	// Tell the calling application the peer has gone
	n.sendEvent(&Event{Type: "EXIT", PeerUUID: peer.Identity, PeerName: peer.Name})

	// Remove peer from any groups we've got it in
	for _, group := range n.peerGroups {
		deletePeer(peer, group)
	}
	// To destroy peer, we remove from peers hash table
	peer.Disconnect()
	delete(n.peers, peer.Identity)
}

// requirePeerGroup finds or creates group via its name
func (n *node) requirePeerGroup(name string) *Group {
	group, ok := n.peerGroups[name]
	if !ok {
		group = NewGroup(name)
		n.peerGroups[name] = group
	}
	return group
}

// joinPeerGroup peer joins group
func (n *node) joinPeerGroup(peer *Peer, name string) *Group {
	group := n.requirePeerGroup(name)
	group.Join(peer)
	// Now tell the caller about the peer joined group
	n.sendEvent(&Event{Type: "JOIN", PeerUUID: peer.Identity,
		PeerName: peer.Name, Group: name})
	return group
}

// leavePeerGroup peer leaves group
func (n *node) leavePeerGroup(peer *Peer, name string) *Group {
	group := n.requirePeerGroup(name)
	group.Leave(peer)
	// Now tell the caller about the peer left group
	n.sendEvent(&Event{Type: "LEAVE", PeerUUID: peer.Identity,
		PeerName: peer.Name, Group: name})
	return group
}

// recvPeer handles messages coming from other peers
func (n *node) recvPeer(msg *ZreMsg) {
	routingID := msg.routingID
	if len(routingID) < 1 {
		return
	}
	// Router socket tells us the identity of this peer
	// Identity must be [1] followed by 16-byte UUID
	identity := getHexUpper(routingID[1:])
	peer := n.peers[identity]
	// On HELLO we may create the peer if it's unknown
	// On other commands the peer must already exist
	if msg.id == HelloID {
		if peer != nil {
			// remove fake peers
			if peer.ready {
				n.removePeer(peer)
			} else if peer.endpoint == n.endpoint {
				// We ignore HELLO, if peer has same endpoint as current node
				return
			}
		}
		var err error
		peer, err = n.requirePeer(identity, msg.Endpoint)
		if err == nil {
			peer.ready = true
		}
	}

	// Ignore command if peer isn't ready
	if peer == nil || !peer.ready {
		if peer != nil {
			n.removePeer(peer)
		}
		return
	}

	// FIXME review this
	if !peer.MessagesLost(msg) {
		log.Printf("%s messages lost from %s", n.name, identity)
		return
	}

	// Now process each command
	switch msg.id {
	case HelloID:
		// Store properties from HELLO command into peer
		peer.Name = msg.Name
		event := &Event{
			Type:     "ENTER",
			PeerUUID: peer.Identity,
			PeerName: peer.Name,
			PeerAddr: peer.endpoint,
			Headers:  make(map[string]string),
		}
		for key, val := range msg.Headers {
			peer.Headers[key] = val
			event.Headers[key] = val
		}
		n.sendEvent(event)
		// Join peer to listed groups
		for _, group := range msg.Groups {
			n.joinPeerGroup(peer, group)
		}
		// Now take peer's status from HELLO, after joining groups
		peer.status = msg.Status
	case WhisperID:
		// Pass up to caller API as WHISPER event
		n.sendEvent(&Event{Type: "WHISPER", PeerUUID: identity,
			PeerName: peer.Name, Msg: msg.Content})
	case ShoutID:
		// Pass up to caller API as SHOUT event
		n.sendEvent(&Event{Type: "SHOUT", PeerUUID: identity,
			PeerName: peer.Name, Group: msg.Group, Msg: msg.Content})
	case PingID:
		ping := NewZreMsg(PingOkID)
		peer.Send(ping)
	case JoinID:
		n.joinPeerGroup(peer, msg.Group)
		if msg.Status != peer.status {
			panic(fmt.Sprintf("[%X] msg.Status != peer.status (%d != %d)", n.uuid, msg.Status, peer.status))
		}
	case LeaveID:
		n.leavePeerGroup(peer, msg.Group)
		if msg.Status != peer.status {
			panic(fmt.Sprintf("[%X] msg.Status != peer.status (%d != %d)", n.uuid, msg.Status, peer.status))
		}
	}
	// Activity from peer resets peer timers
	peer.Refresh()
}

func (n *node) recvBeacon(s *Signal) {
	b := &BeaconMsg{}
	b.unpack(s.Transmit)

	// Ignore anything that isn't a valid beacon
	if b.Version == beaconVersion {
		identity := getHexUpper(b.UUID)
		if b.Port != 0 {
			// Check that the peer, identified by its UUID, exists
			peer, err := n.requirePeer(identity, getEndpoint(s.Addr, b.Port))
			if err == nil {
				peer.Refresh()
			} else if n.verbose {
				log.Printf("%s %s", n.name, err)
			}
		} else {
			// Zero port means peer is going away; remove it if
			// we had any knowledge of it already
			peer := n.peers[identity]
			n.removePeer(peer)
		}
	} else if n.verbose {
		log.Printf("invalid ZRE Beacon version %d", b.Version)
	}
}

// We do this once a second:
// - if peer has gone quiet, send TCP ping and emit EVASIVE event
// - if peer has disappeared, expire it
func (n *node) pingPeer(peer *Peer) {
	if time.Now().Unix() >= peer.expiredAt.Unix() {
		n.removePeer(peer)
	} else if time.Now().Unix() >= peer.evasiveAt.Unix() {
		// If peer is being evasive, force a TCP ping.
		// TODO: do this only once for a peer in this state;
		// it would be nicer to use a proper state machine
		// for peer management.
		m := NewZreMsg(PingID)
		peer.Send(m)
	}
}

func (n *node) actor() {
	defer n.wg.Done()
	tick := time.Tick(reapInterval)
	for {
		select {
		case r := <-n.requests:
			n.recvAPI(r)
		case m, ok := <-n.msgs:
			if !ok {
				//	clear peer tables
				for peerID, peer := range n.peers {
					peer.Disconnect()
					delete(n.peers, peerID)
				}
				return
			}
			n.recvPeer(m)
		case s := <-n.beacon.Signals:
			n.recvBeacon(s)
		case <-tick:
			for _, peer := range n.peers {
				n.pingPeer(peer)
			}
		}
	}
}

func (n *node) sendEvent(e *Event) {
	select {
	case n.events <- e:
	default:
		if n.verbose {
			log.Printf("event channel is full, dropping: %s", e.Type)
		}
	}
}
