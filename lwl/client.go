// Package lwl implements a service for authorising and communicating with a
// LightwaveRF Link (LWL)
package lwl

import (
	"fmt"
	"net"
	"strings"
	"sync/atomic"
	"time"
)

const lwlServerPort = 9760 // We send to this address ...
const lwlClientPort = 9761 // ... and listen for responses on this one

// Client implements a communication channel with LightwaveRF Link (LWL)
type Client struct {
	sid atomic.Int32

	// Discovered at runtime
	addr net.UDPAddr      // Unicast address of LWL
	mac  net.HardwareAddr // MAC address of LWL

	rx chan string // Queue of messages from LWL -> Us
	tx chan string // Queue of requests from Us -> LWL
}

// New returns a Client
func New() *Client {
	c := Client{
		sid: atomic.Int32{},
		addr: net.UDPAddr{
			IP:   net.IPv4bcast,
			Port: lwlServerPort,
		},
		rx: make(chan string, 16),
		tx: make(chan string, 16),
	}
	return &c
}

// Listen captures traffic from the LWL and writes it into the given channel
func (c *Client) Listen(out chan<- string) error {
	addr := net.UDPAddr{Port: lwlClientPort}
	con, err := net.ListenUDP("udp", &addr)
	if err != nil {
		return err
	}
	var msg = make([]byte, 1024)
	for {
		i, err := con.Read(msg)
		if err != nil {
			return err
		}

		println("debug", i, string(msg[:i]))
		out <- string(msg[:i])
	}
}

// Send transmits a payload to the LWL, and waits for an acknowledgement
func (c *Client) Send(payload string) error {
	var out []string
	if len(c.mac) > 0 {
		out = append(out, fmt.Sprintf("%x%x%x,",
			c.mac[len(c.mac)-3],
			c.mac[len(c.mac)-2],
			c.mac[len(c.mac)-1],
		))
	}
	out = append(out, fmt.Sprintf("%d", (c.sid.Add(1)))) // Generate new sequence identifier
	out = append(out, payload)

	msg := strings.Join(out, ",")

	conn, err := net.Dial("udp", c.addr.String())
	if err != nil {
		return err
	}
	defer conn.Close()

	conn.SetReadDeadline(time.Now().Add(time.Second * 3))

	println("send-tx:", msg)
	conn.Write([]byte(msg))

	b := make([]byte, 1024)
	i, err := conn.Read(b)
	if err != nil {
		return err
	}
	println("send-rx:", b[:i])

	// TODO: Wait for response from LWL (either OK, or error)

	return nil
}
