// Package lwl implements a service for authorising and communicating with a
// LightwaveRF Link (LWL)
package lwl

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/davecgh/go-spew/spew"
)

const lwlServerPort = 9760 // We send to this address ...
const lwlClientPort = 9761 // ... and listen for responses on this one

// Response holds a decoded JSON message from the LWL.
// e.g. *!{"trans":12090,"mac":"20:3B:85","time":1766967067,"pkt":"error","fn":"nonRegistered","payload":"Not yet registered. See LightwaveLink"}
type Response struct {
	Trans   int    `json:"trans"`
	Mac     string `json:"mac"`
	Time    int    `json:"time"`
	Pkt     string `json:"pkt"`
	Fn      string `json:"fn"`
	Payload string `json:"payload"`
}

// Client implements a communication channel with LightwaveRF Link (LWL)
type Client struct {
	sid atomic.Int32 // Sequence ID

	// Discovered at runtime
	addr net.UDPAddr // Unicast address of LWL
	mac  string      // MAC address of LWL

	rx chan string // Queue of messages from LWL -> Us
	tx chan string // Queue of requests from Us -> LWL

	con *net.UDPConn // UDP connection for LAN traffic

	// Protects pending
	lock sync.RWMutex
	// Outstanding transactions keyed on sid prefix expected in LWL reply
	pending map[string]chan string
}

// New returns a Client
func New() *Client {
	con, err := net.ListenUDP("udp4", &net.UDPAddr{Port: lwlClientPort})
	if err != nil {
		panic(err)
	}

	c := Client{
		sid: atomic.Int32{},
		addr: net.UDPAddr{
			IP: net.IPv4bcast,
			// IP:   net.ParseIP("192.168.4.71"),
			Port: lwlServerPort,
		},
		rx:  make(chan string, 16),
		tx:  make(chan string, 16),
		con: con,

		lock:    sync.RWMutex{},
		pending: make(map[string]chan string),
	}
	return &c
}

// Register stores a channel into which legacy messages from the LWL will be
// written if they match the given sequence id
func (c *Client) Register(sid string, ch chan string) {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.pending[sid] = ch
}

// Deregister undoes Register
func (c *Client) Deregister(sid string) {
	c.lock.Lock()
	defer c.lock.Unlock()
	delete(c.pending, sid)
}

// Listen captures traffic from the LWL and writes it into the given channel
func (c *Client) Listen(out chan<- string) {
	var b = make([]byte, 1024)
	for {
		i, err := c.con.Read(b)
		if err != nil {
			if errors.Is(err, os.ErrDeadlineExceeded) {
				continue
			}
			panic(err)
		}

		msg := string(b[:i])
		println("listen:", msg)

		if strings.HasPrefix(msg, "*") {
			// JSON response
			// e.g. *!{"trans":12090,"mac":"20:3B:85","time":1766967067,"pkt":"error","fn":"nonRegistered","payload":"Not yet registered. See LightwaveLink"}
			println("json")
			if i < 2 {
				println("ERROR: Invalid JSON from LWL, not long enough!", msg)
				continue
			}
			var r Response
			err := json.Unmarshal(b[2:i], &r)
			if err != nil {
				println("ERROR: Failed to parse JSON:", err.Error())
				continue
			}
			spew.Dump(r)
			if len(c.mac) == 0 && len(r.Mac) > 0 {
				c.mac = r.Mac
			}
		} else {
			// Legacy response
			// e.g. ERR,1,"Not yet registered. Send !F*p to register"
			println("legacy")
			sid, _, found := strings.Cut(msg, ",")
			if !found {
				println("WARNING: Unable to parse legacy message:", msg)
			} else {
				c.lock.RLock()
				waiter, ok := c.pending[sid]
				c.lock.Unlock()
				spew.Dump(waiter, ok)
				if ok {
					waiter <- msg
				}
			}
		}
		out <- msg
	}
}

// Send transmits a payload to the LWL, and returns the sequence ID (sid) of
// the request. The sid can be used to identify error responses (non-JSON).
func (c *Client) Send(payload string) string {
	var out []string

	// Generate new sid, atomically
	sid := fmt.Sprintf("%d", c.sid.Add(1))

	if len(c.mac) > 0 {
		out = append(out, fmt.Sprintf(":%s", c.mac))
	}
	out = append(out, sid)
	out = append(out, payload)

	msg := strings.Join(out, ",")

	c.con.WriteToUDP([]byte(msg), &c.addr)

	return sid
}

// DoLegacy sends a given payload, and then waits for a non-JSON response from
// the LWL
func (c *Client) DoLegacy(payload string) string {
	sid := c.Send(payload)
	waiter := make(chan string)

	c.Register(sid, waiter)
	defer c.Deregister(sid)

	select {
	case reply := <-waiter:
		return reply
	case <-time.After(time.Second * 3):
		return ""
	}
}
