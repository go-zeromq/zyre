// Copyright 2020 The go-zeromq Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package zyre

import (
	"fmt"
	"net"
	"strconv"

	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
)

// Connection defines a common interface for ipv4 and ipv6
type Connection interface {
	Close() error
	Addr() string
	JoinGroup(ifi *net.Interface) error
	ReadFrom(buff []byte) (n int, src net.IP, err error)
	WriteTo(buff []byte) error
}

// ConnData common data for ipv4 and ipv6
type ConnData struct {
	port    int
	addr    string
	outAddr *net.UDPAddr
}

// joinGroup ...
func (conn *ConnData) joinGroup(ifi *net.Interface,
	validIP func(*net.IP) bool) error {
	addrs, err := ifi.Addrs()
	if err != nil {
		return err
	}

	for _, addr := range addrs {
		ip, _, err := net.ParseCIDR(addr.String())
		if err != nil {
			return err
		}
		if validIP(&ip) {
			conn.addr = ip.String()
			break
		}
	}
	if conn.addr == "" {
		return fmt.Errorf("no valid address in interface %s", ifi.Name)
	}
	return err
}

// Addr ...
func (conn *ConnData) Addr() string {
	return conn.addr
}

// IPv4Conn IPv4 connection
type IPv4Conn struct {
	ConnData
	c *ipv4.PacketConn
}

// IPv6Conn IPv6 connection
type IPv6Conn struct {
	ConnData
	c *ipv6.PacketConn
}

// NewIPv4Conn returns a new IPv4Conn
func NewIPv4Conn(port int) (*IPv4Conn, error) {
	conn := &IPv4Conn{ConnData: ConnData{port: port}}
	addr := net.JoinHostPort(net.IPv4allsys.String(), strconv.Itoa(port))
	c, err := net.ListenPacket("udp4", addr)
	if err == nil {
		conn.c = ipv4.NewPacketConn(c)
		conn.c.SetMulticastLoopback(true)
		conn.c.SetControlMessage(ipv4.FlagSrc, true)
	}
	return conn, err
}

// NewIPv6Conn returns a new IPv6Conn
func NewIPv6Conn(port int) (*IPv6Conn, error) {
	conn := &IPv6Conn{ConnData: ConnData{port: port}}
	addr := net.JoinHostPort(net.IPv6linklocalallnodes.String(), strconv.Itoa(port))
	c, err := net.ListenPacket("udp6", addr)
	if err == nil {
		conn.c = ipv6.NewPacketConn(c)
		conn.c.SetMulticastLoopback(true)
		conn.c.SetControlMessage(ipv6.FlagSrc, true)
	}
	return conn, err
}

// NewConn returns a new Connection
func NewConn(port int) (Connection, error) {
	var conn Connection
	conn, err := NewIPv4Conn(port)
	if err != nil {
		conn, err = NewIPv6Conn(port)
	}
	return conn, err
}

// Close ...
func (conn *IPv4Conn) Close() error {
	return conn.c.Close()
}

// Close ...
func (conn *IPv6Conn) Close() error {
	return conn.c.Close()
}

// JoinGroup ..
func (conn *IPv4Conn) JoinGroup(ifi *net.Interface) error {
	err := conn.c.JoinGroup(ifi, &net.UDPAddr{IP: net.IPv4allsys})
	if err != nil {
		return err
	}
	err = conn.joinGroup(ifi, func(ip *net.IP) bool { return ip.To4() != nil })
	if err != nil {
		return err
	}
	conn.outAddr = &net.UDPAddr{IP: net.IPv4bcast, Port: conn.port}
	if ifi.Flags&net.FlagBroadcast == 0 {
		conn.outAddr = &net.UDPAddr{IP: net.IPv4allsys, Port: conn.port}
	}
	return err
}

// JoinGroup ..
func (conn *IPv6Conn) JoinGroup(ifi *net.Interface) error {
	err := conn.c.JoinGroup(ifi, &net.UDPAddr{IP: net.IPv6linklocalallnodes})
	if err != nil {
		return err
	}
	err = conn.joinGroup(ifi, func(ip *net.IP) bool { return ip.To16() != nil })
	if err != nil {
		return err
	}
	conn.outAddr = &net.UDPAddr{IP: net.IPv6linklocalallnodes, Port: conn.port}
	return err
}

// ReadFrom ...
func (conn *IPv4Conn) ReadFrom(buff []byte) (int, net.IP, error) {
	var cm *ipv4.ControlMessage
	var addr net.IP
	n, cm, _, err := conn.c.ReadFrom(buff)
	if err == nil && n < beaconMax && n != 0 && cm != nil {
		addr = cm.Src
	}
	return n, addr, err
}

// ReadFrom ...
func (conn *IPv6Conn) ReadFrom(buff []byte) (int, net.IP, error) {
	var cm *ipv6.ControlMessage
	var addr net.IP
	n, cm, _, err := conn.c.ReadFrom(buff)
	if err == nil && n < beaconMax && n != 0 && cm != nil {
		addr = cm.Src
	}
	return n, addr, err
}

// WriteTo ...
func (conn *IPv4Conn) WriteTo(buff []byte) error {
	_, err := conn.c.WriteTo(buff, nil, conn.outAddr)
	return err
}

// WriteTo ...
func (conn *IPv6Conn) WriteTo(buff []byte) error {
	_, err := conn.c.WriteTo(buff, nil, conn.outAddr)
	return err
}
