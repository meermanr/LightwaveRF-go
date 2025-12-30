// Package lwl implements (10 * time.Second)a service for authorising and communicating with a
// LightwaveRF Link (LWL)
package lwl

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/davecgh/go-spew/spew"
)

type VMutex struct {
	m sync.Mutex
}

func newVMutex() VMutex {
	v := VMutex{
		m: sync.Mutex{},
	}
	return v
}

func (v *VMutex) Lock() {
	println("Lock")
	debug.PrintStack()
	v.m.Lock()
}
func (v *VMutex) Unlock() {
	println("Unlock")
	debug.PrintStack()
	v.m.Unlock()
}

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
// e.g. *!{"trans":13367,"mac":"20:3B:85","time":1767129960,"type":"link","prod":"lwl","pairType":"local","msg":"success","class":"","serial":""}
type Response struct {
	Trans int    `json:"trans"`
	Mac   string `json:"mac"`
	Time  int    `json:"time"`

	Pkt     string `json:"pkt"`
	Fn      string `json:"fn"`
	Payload string `json:"payload"`

	Type     string `json:"type"`
	Prod     string `json:"prod"`
	PairType string `json:"pairType"`
	Msg      string `json:"msg"`
	Class    string `json:"class"`
	Serial   string `json:"serial"`
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
	pendingLock sync.Mutex

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
		pendingLock:   sync.Mutex{},

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
			println("listen.parseJSON:", err.Error())

			sid, payload, err := c.parseLegacy(msg)
			if err != nil {
				println("WARNING: Unable to parse legacy message:", msg)
				continue
			}
			// Legacy response
			// e.g. ERR,1,"Not yet registered. Send !F*p to register"

			c.pendingLock.Lock()
			waiter, ok := c.pendingLegacy[sid]
			c.pendingLock.Unlock()
			spew.Dump(waiter, ok)
			if ok {
				waiter <- payload
			}
		} else {

			// Valid message, we'll talk to this LWL from now on
			c.addr.IP = addr.IP

			// Feed message to subscribers, if able
			c.pendingLock.Lock()
			for _, chr := range c.pendingJSON {
				select {
				case chr <- r:
				default:
					// Means we were unable to write to the channel (full?)
				}
			}
			c.pendingLock.Unlock()

			// Feed message to user, or discard if unable
			select {
			case out <- r:
			default:
			}
		}
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

func (c *Client) sendRaw(msg string) {
	c.sendLock.Lock()
	c.con.WriteToUDP([]byte(msg), &c.addr)
	time.Sleep(100 * time.Millisecond)
	c.sendLock.Unlock()

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

	c.sendRaw(msg)

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

	chr := make(chan Response, 10)
	chs := make(chan string, 10)
	sid := c.Send(payload, chr, chs)

	defer c.Unsubscribe(sid)

	t := time.NewTimer(time.Second)
	pairingRequired := true

	for pairingRequired == true {
		select {
		case r := <-chr:
			// println("[PAIRING] LWL:", spew.Sdump(r))
			switch {
			// *!{"trans":13366,"mac":"20:3B:85","time":1767129953,"pkt":"error","fn":"nonRegistered","payload":"Not yet registered. See LightwaveLink"}

			case r.Fn == "nonRegistered":
				pairingRequired = true
				println("Pairing required: Please press button on LightwaveLink")
			// *!{"trans":13367,"mac":"20:3B:85","time":1767129960,"type":"link","prod":"lwl","pairType":"local","msg":"success","class":"","serial":""}
			case r.PairType == "local" && r.Msg == "success":
				pairingRequired = false
				println("Pairing successful")
			}
		case s := <-chs:
			// E.g. ?V="N2.94D"
			// println("[PAIRING]", s)
			if strings.HasPrefix(s, "?V=") {
				println("Already paired with LightwaveLink", strings.TrimSpace(s))
				pairingRequired = false
			}
		case <-t.C:
			println("Timeout. Resending pairing request")
			c.sendRaw(fmt.Sprintf("%s,%s", sid, payload))
			t.Reset(10 * time.Second) // LWL pairing ends after ~15s
		}
	}
}
