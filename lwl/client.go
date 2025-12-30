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

type errNotJSON struct {
	msg string
}

func (e errNotJSON) Error() string {
	return e.msg
}

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

	con *net.UDPConn // UDP connection for LAN traffic

	// Outstanding transactions keyed on sid. Legacy format messages from the LWL
	// with a matching sid will be written to the channel. Use Subscribe() to
	// add, Unsubscribe() to remove.
	pendingJSON   map[string]chan Response
	pendingLegacy map[string]chan string
	// Protects pending
	pendingLock sync.RWMutex

	// Serialises transmission
	sendLock sync.Mutex
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
			IP:   net.IPv4bcast,
			Port: lwlServerPort,
		},
		con: con,

		pendingJSON:   make(map[string]chan Response),
		pendingLegacy: make(map[string]chan string),
		pendingLock:   sync.RWMutex{},

		sendLock: sync.Mutex{},
	}
	return &c
}

// Subscribe stores a channel into which legacy messages from the LWL will be
// written if they match the given sequence id, i.e. error responses to
// commands.
func (c *Client) Subscribe(sid string, chr chan Response, chs chan string) {
	c.pendingLock.Lock()
	defer c.pendingLock.Unlock()
	c.pendingJSON[sid] = chr
	c.pendingLegacy[sid] = chs
}

// Unsubscribe undoes Subscribe()
func (c *Client) Unsubscribe(sid string) {
	c.pendingLock.Lock()
	defer c.pendingLock.Unlock()
	delete(c.pendingJSON, sid)
	delete(c.pendingLegacy, sid)
}

// Render internal state as a string
func (c *Client) String() string {
	return spew.Sprintf(`
lwl.Client(
  sid:     %v
  addr:    %v
  mac:     %v
  pending: %v
)
`,
		c.sid.Load(),
		c.addr,
		c.mac,
		c.pendingLegacy,
	)
}

// Listen captures traffic from the LWL and writes it into the given channel
func (c *Client) Listen(out chan<- Response) {
	var b = make([]byte, 1024)
	for {
		i, addr, err := c.con.ReadFromUDP(b)
		if err != nil {
			if errors.Is(err, os.ErrDeadlineExceeded) {
				continue
			}
			panic(err)
		}

		msg := string(b[:i])

		r, err := c.parseJSON(msg)
		if err != nil {
			// Not JSON, maybe legacy response?

			sid, payload, err := c.parseLegacy(msg)
			if err != nil {
				println("WARNING: Unable to parse legacy message:", msg)
				continue
			}
			// Legacy response
			// e.g. ERR,1,"Not yet registered. Send !F*p to register"

			c.pendingLock.RLock()
			waiter, ok := c.pendingLegacy[sid]
			c.pendingLock.RUnlock()
			spew.Dump(waiter, ok)
			if ok {
				waiter <- payload
			}
		}

		// Valid message, we'll talk to this LWL from now on
		c.addr.IP = addr.IP

		// Feed message to subscribers
		c.pendingLock.RLock()
		for _, chr := range c.pendingJSON {
			chr <- r
		}
		c.pendingLock.RUnlock()
		// Feed message to user
		out <- r
	}
}

// Parse JSON response
func (c *Client) parseJSON(msg string) (Response, error) {
	if !strings.HasPrefix(msg, "*") {
		return Response{}, errNotJSON{msg: "not JSON: Does not start with literal asterisk"}
	}
	// JSON response
	// e.g. *!{"trans":12090,"mac":"20:3B:85","time":1766967067,"pkt":"error","fn":"nonRegistered","payload":"Not yet registered. See LightwaveLink"}
	if len(msg) < 2 {
		return Response{}, errors.New("invalid JSON: Message not long enough")
	}

	b := []byte(msg)
	var r Response
	err := json.Unmarshal(b[2:], &r)
	if err != nil {
		return r, fmt.Errorf("failed to parse JSON: %w", err)
	}
	return r, nil
}

func (c *Client) parseLegacy(msg string) (string, string, error) {
	// Legacy response
	// e.g. ERR,1,"Not yet registered. Send !F*p to register"
	sid, payload, found := strings.Cut(msg, ",")
	if !found {
		return "", "", fmt.Errorf("unable to parse legacy message: %v", msg)
	}
	return sid, payload, nil
}

// Send transmits a payload to the LWL, and returns the sequence ID (sid) of
// the request. If a non-nil channel is provided, it will be subscribed to
// replies; the caller is responsible for calling Unsubscribe().
func (c *Client) Send(payload string, chr chan Response, chs chan string) string {
	var out []string

	// Generate new sid, atomically
	sid := fmt.Sprintf("%d", c.sid.Add(1))

	if len(c.mac) > 0 {
		out = append(out, fmt.Sprintf(":%s", c.mac))
	}
	out = append(out, sid)
	out = append(out, payload)

	msg := strings.Join(out, ",")

	if chr != nil && chs != nil {
		c.Subscribe(sid, chr, chs)
	}

	c.sendLock.Lock()
	c.con.WriteToUDP([]byte(msg), &c.addr)
	time.Sleep(100 * time.Millisecond)
	c.sendLock.Unlock()

	return sid
}

// DoLegacy sends a given payload, and then waits for a non-JSON response from
// the LWL
func (c *Client) DoLegacy(payload string) string {
	chr := make(chan Response)
	chs := make(chan string)
	sid := c.Send(payload, chr, chs)

	defer c.Unsubscribe(sid)

	select {
	case reply := <-chr:
		spew.Dump(reply)
		return ""
	case reply := <-chs:
		return reply
	case <-time.After(time.Second):
		return ""
	}
}

// EnsureRegistered checks if the LWL accepts commands from the current host,
// and if not begins pairing mode.
func (c *Client) EnsureRegistered() {
	payload := "!F*p" // Request pairing (or if already paired, requests LWL version)

	chr := make(chan Response)
	chs := make(chan string)
	sid := c.Send(payload, chr, chs)

	defer c.Unsubscribe(sid)

	t := time.NewTimer(time.Second * 30)
	pairingRequired := false
	done := false

	for done != true {
		select {
		case r := <-chr:
			println("[PAIRING] LWL:", spew.Sdump(r))
			if r.Fn == "nonRegistered" {
				pairingRequired = true
				done = true
			}
		case s := <-chs:
			// E.g. ?V="N2.94D"
			println("Already paired with LightwaveLink", strings.TrimSpace(s))
			if strings.HasPrefix(s, "?V=") {
				pairingRequired = false
				done = true
			}
		case <-t.C:
			println("Timeout. Resending pairing request")
			// Cleanup
			c.Unsubscribe(sid)
			// New pairing request
			sid := c.Send(payload, chr, chs)
			defer c.Unsubscribe(sid)
		}
	}

	if pairingRequired {
		println("Pairing required: Please press button on LightwaveLink")
	}
}
