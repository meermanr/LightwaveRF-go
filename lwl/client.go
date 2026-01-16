// Package lwl implements (10 * time.Second)a service for authorising and communicating with a
// LightwaveRF Link (LWL)
package lwl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
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

// Response holds a decoded JSON message from the LWL. Not all fields are used
// by all LWL messages.
//
// Sample JSON:
//
//	*!{"trans":12090,"mac":"20:3B:85","time":1766967067,"pkt":"error","fn":"nonRegistered","payload":"Not yet registered. See LightwaveLink"}
//	*!{"trans":13367,"mac":"20:3B:85","time":1767129960,"type":"link","prod":"lwl","pairType":"local","msg":"success","class":"","serial":""}
//	*!{"trans":14619,"mac":"20:3B:85","time":1767288212,"pkt":"system","fn":"hubCall","type":"hub","prod":"lwl","fw":"N2.94D","uptime":2790197,"timeZone":0,"lat":52.18,"long":0.21,"tmrs":1,"evns":5,"run":0,"macs":1,"ip":"192.168.4.71","devs":11}
//	*!{"trans":14674,"mac":"20:3B:85","time":1767297488,"pkt":"room","fn":"summary","stat0":255,"stat1":7,"stat2":0,"stat3":0,"stat4":0,"stat5":0,"stat6":0,"stat7":0,"stat8":0,"stat9":0}
//	*!{"trans":14819,"mac":"20:3B:85","time":1767307528,"pkt":"room","fn":"read","slot":10,"serial":"D88002","prod":"valve"}
type Response struct {
	// Common to all
	Trans int32  `json:"trans"` // Transaction number of the source JSON packet. Increments every transaction. Not related to sid.
	Mac   string `json:"mac"`   // Last 6 octets of LightwaveLink MAC address, e.g."20:3B:85"
	Time  int32  `json:"time"`  // Timestamp of the transaction in LWL "local" Unixtime (i.e. if Link is set to UTC+2, this time will be UNIX + (3600*2))

	// errors
	Pkt     string `json:"pkt"` // Packet. "system", "error", "433T" to indicate a 433MHz transmission (i.e. LWL to Device), or "868R" to indicate 868MHz radio being received
	Fn      string `json:"fn"`  // Function. "error", "system", "on", "off", "dim", "fullLock", "manualLock", "unlock", "open", "close", "stop", "ledColour", "ledColourCycle", "allOff", "moodStore", "moodRecall", "read"
	Payload string `json:"payload"`

	// pkt:433T (LWL stating that it is sending a command to a device via 433 MHz transmission)
	Room  string `json:"room"`  // The room number that the command was sent to, 0-80 (inc.)
	Dev   string `json:"dev"`   // The device number that the command was sent to
	Param string `json:"Param"` // Not in every packet. The parameter for the function, if the function requires a parameter (i.e. dim, mood slot)

	// type:link (e.g. !F*p)
	Type     string `json:"type"`     // "link" or "unlink"
	Prod     string `json:"prod"`     // wfl=WiFiLink (has a screen), lwl=LightwaveLink (no screen), LW920=Boiler Switch, tmr1ch=LW921=Home Thermostat (Timer 1 Channel), valve=LW922=Thermostatic Radiator Valve (TRV), electr=LW934=Electric Switch
	PairType string `json:"pairType"` // "local" (LAN to hub), "product" (new device, e.g. TRV)
	Msg      string `json:"msg"`
	Class    string `json:"class"`
	Serial   string `json:"serial"` // Identifies energy/heating device.

	// type:hub (e.g. @H, hubCall)
	Fw       string  `json:"fw"`       // Which firmware build the Link is running
	Uptime   int32   `json:"uptime"`   // The time in seconds that the Link has been running for without a power cycle, or software restart
	Timezone int32   `json:"timeZone"` // The current timezone of the Link in GMT (the Link automatically goes into DST). 0 is GMT, while 1 is GMT+1 and -5 is GMT-5
	Lat      float32 `json:"lat"`      // Latitude of the Links location for dusk/dawn calculations
	Long     float32 `json:"long"`     // Longitude of the Links location for dusk/dawn calculations
	Timers   int32   `json:"tmrs"`     // The number of Timers stored in the Link
	Events   int32   `json:"evns"`     // The number of Events stored in the Link
	Macs     int32   `json:"macs"`     // The number of tablets/phones/PCs the Link is paired to
	IP       string  `json:"ip"`       // The local IP address the Link is using
	Devs     int32   `json:"devs"`     // The number of heating and energy devices the Link is currently paired to\
	DawnTime int32   `json:"dawnTime"` // "Local" unixtime of dawn
	DuskTime int32   `json:"duskTime"` // "Local" unixtime of dusk

	// pkt:room
	Stat0 uint8 `json:"stat0"` // Bitfile indicating which slots are in use. LSB=R0, MSB=R8
	Stat1 uint8 `json:"stat1"` // Bitfile indicating which slows are in use. LSB=R9, MSB=R16
	Stat2 uint8 `json:"stat2"` // Bitfile indicating which slows are in use. LSB=R17, MSB=R24
	Stat3 uint8 `json:"stat3"` // Bitfile indicating which slows are in use. LSB=R25, MSB=R32
	Stat4 uint8 `json:"stat4"` // Bitfile indicating which slows are in use. LSB=R33, MSB=R40
	Stat5 uint8 `json:"stat5"` // Bitfile indicating which slows are in use. LSB=R41, MSB=R48
	Stat6 uint8 `json:"stat6"` // Bitfile indicating which slows are in use. LSB=R49, MSB=R56
	Stat7 uint8 `json:"stat7"` // Bitfile indicating which slows are in use. LSB=R57, MSB=R64
	Stat8 uint8 `json:"stat8"` // Bitfile indicating which slows are in use. LSB=R65, MSB=R72
	Stat9 uint8 `json:"stat9"` // Bitfile indicating which slows are in use. LSB=R73, MSB=R80

	// Internal
	json string // Original message, before it was decoded
}

func (r *Response) String() string {
	return r.json
}

// LogValue implements slog.LogValuer.
func (r Response) LogValue() slog.Value {
	return slog.StringValue(r.String())
}

// Client implements a communication channel with LightwaveRF Link (LWL)
type Client struct {
	sid atomic.Int32 // Sequence ID (we tag our commands with this, so we can recognise replies)

	// The hub will send replies to command twice: once unicast (i.e. direct to
	// us) and again via broadcast. We track the transaction number so we can
	// discard duplicates.

	tid atomic.Int32 // Transaction ID (hub monotonically increases this in JSON responses)

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

	// Metrics
	latencyStatsLock sync.Mutex
	latencyStats     map[string]*LatencyStats
}

// New returns a Client
func New() *Client {
	con, err := net.ListenUDP("udp4", &net.UDPAddr{Port: lwlClientPort})
	if err != nil {
		panic(err)
	}

	c := Client{
		addr: net.UDPAddr{
			IP:   net.IPv4bcast,
			Port: lwlServerPort,
		},
		con: con,

		pendingJSON:   make(map[string]chan Response),
		pendingLegacy: make(map[string]chan string),
		latencyStats:  make(map[string]*LatencyStats),
	}
	return &c
}

// Subscribe to Response and (if sid is non-empty) ACK/NACK messages.
//
// Returns a sequence ID which can be used with Unsubscribe.
//
// If the input sid is an empty string, one will be allocated.
func (c *Client) Subscribe(sid string, chr chan Response, chs chan string) string {
	if len(sid) == 0 {
		sid = fmt.Sprintf("%d", c.sid.Add(1))
	}
	c.pendingLock.Lock()
	defer c.pendingLock.Unlock()
	c.pendingJSON[sid] = chr
	c.pendingLegacy[sid] = chs
	return sid
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
  sid:           %v
  addr:          %v
  pendingJSON:   %v
  pendingLegacy: %v
)
`,
		c.sid.Load(),
		c.addr,
		c.pendingJSON,
		c.pendingLegacy,
	)
}

// Listen captures traffic from the LWL and writes it to all subscribers
func (c *Client) Listen() {
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

		if errJSON := c.handleJSON(msg); errJSON != nil {
			if _, ok := errJSON.(errNotJSON); ok {
				// Not JSON. Try legacy
				if errLegacy := c.handleLegacy(msg); errLegacy != nil {
					// Uh-ho. No idea what this is
					slog.Warn("Unable to parse message as either JSON or Legacy:",
						"msg", msg,
						"errJSON", errJSON,
						"errLegacy", errLegacy,
					)
					continue // Abandon processing of this message
				}
			} else {
				// Was JSON, but invalid in some way
				slog.Error("Bad JSON", "errJSON", errJSON)
			}
		}

		// Valid message, we'll talk to this LWL from now on
		c.addr.IP = addr.IP
	}
}

// handleJSON decodes a message into a Response, and writes it to all subscribers
func (c *Client) handleJSON(msg string) error {
	r, err := c.parseJSON(msg)
	if err != nil {
		return err
	}

	if r.Trans <= c.tid.Load() {
		// Duplicate message, discard
		return nil
	}

	// Record that we've seen this transaction ID
	c.tid.Store(r.Trans)

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

	return nil
}

// Legacy response
// e.g. ERR,1,"Not yet registered. Send !F*p to register"
func (c *Client) handleLegacy(msg string) error {
	// Not JSON, maybe legacy response?
	sid, payload, err := c.parseLegacy(msg)
	if err != nil {
		return err
	}

	// Write message to legacy subscribers
	c.pendingLock.Lock()
	waiter, ok := c.pendingLegacy[sid]
	c.pendingLock.Unlock()
	if ok {
		// Non-blocking write to channel
		select {
		case waiter <- payload:
		default:
		}
	}
	return nil
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
	r.json = msg
	return r, nil
}

func (c *Client) parseLegacy(msg string) (string, string, error) {
	// Legacy response
	// e.g. ERR,1,"Not yet registered. Send !F*p to register"
	sid, payload, found := strings.Cut(msg, ",")
	if !found {
		return "", "", fmt.Errorf("unable to parse legacy message: %v", msg)
	}
	payload = strings.TrimSpace(payload)
	return sid, payload, nil
}

func (c *Client) sendRaw(msg string) {
	c.sendLock.Lock()
	c.con.WriteToUDP([]byte(msg), &c.addr)
	slog.Debug("sendRaw", "msg", msg)
	// Rate limit sending, to avoid collisions
	go func() {
		// Typical response time is ~25-30ms (from WriteToUDP() returning to
		// c.Listen() picking up a JSON response), but the LWL seems to be unable
		// to process requests faster than every 100ms.
		time.Sleep(125 * time.Millisecond)
		c.sendLock.Unlock()
	}()

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

func (c *Client) sampleCommandLatency(cmd Command, t time.Duration) {
	c.latencyStatsLock.Lock()
	defer c.latencyStatsLock.Unlock()

	ls, ok := c.latencyStats[cmd.cmd]
	if !ok {
		ls = NewLatencyStats(cmd.cmd)
		c.latencyStats[cmd.cmd] = ls
	}
	ls.Sample(t)
}

// Stats reports the min/mean/max times for seen commands to get a (non-error)
// response from the LWL.
//
// The report is intended for human consumption.
func (c *Client) Stats() string {
	c.latencyStatsLock.Lock()
	defer c.latencyStatsLock.Unlock()

	s := make([]string, len(c.latencyStats))

	for _, v := range c.latencyStats {
		s = append(s, v.String())
	}

	out := strings.Join(s, "\n")
	return out
}

// Do performs a command and returns the response, or an error.
func (c *Client) Do(ctx context.Context, cmd Command) (Response, error) {
	chr := make(chan Response, 10)
	chs := make(chan string, 10)
	sid := c.Send(cmd.String(), chr, chs)
	defer c.Unsubscribe(sid)

	// Send() is rate-limited, but returns as soon as transmission is complete,
	// so start timing from when it returns.
	start := time.Now()

	select {
	case msg := <-chs:
		slog.Debug("Do", "msg", &msg)
		if strings.TrimSpace(msg) != "OK" {
			return Response{}, fmt.Errorf("Unexpected (legacy) response to command: %s", msg)
		}
	case r := <-chr:
		slog.Debug("Do", "r", &r)
		if cmd.IsResponse(r) {
			slog.Debug("Do", "r", &r)
			c.sampleCommandLatency(cmd, time.Since(start))
			return r, nil
		}
	case <-ctx.Done():
		return Response{}, ctx.Err()
	}

	return Response{}, nil
}

// EnsureRegistered checks if the LWL accepts commands from the current host,
// and if not begins pairing mode.
func (c *Client) EnsureRegistered() {
	chr := make(chan Response, 10)
	chs := make(chan string, 10)
	sid := c.Send(CmdRegister.String(), chr, chs)

	defer c.Unsubscribe(sid)

	t := time.NewTimer(time.Second)
	pairingRequired := true

	for pairingRequired == true {
		select {
		case r := <-chr:
			slog.Debug("Pairing JSON response", "r", r)
			switch {
			// *!{"trans":13366,"mac":"20:3B:85","time":1767129953,"pkt":"error","fn":"nonRegistered","payload":"Not yet registered. See LightwaveLink"}

			case r.Fn == "nonRegistered":
				pairingRequired = true
				slog.Info("Pairing required: Please press button on LightwaveLink")
			// *!{"trans":13367,"mac":"20:3B:85","time":1767129960,"type":"link","prod":"lwl","pairType":"local","msg":"success","class":"","serial":""}
			case r.PairType == "local" && r.Msg == "success":
				pairingRequired = false
				slog.Info("Pairing successful")
			}
		case s := <-chs:
			// E.g. ?V="N2.94D"
			slog.Debug("Pairing legacy message", "s", s)
			if strings.HasPrefix(s, "?V=") {
				slog.Info("Already paired with LightwaveLink", "s", s)
				pairingRequired = false
			}
		case <-t.C:
			slog.Debug("Timeout. Resending pairing request")
			c.sendRaw(fmt.Sprintf("%s,%v", sid, CmdRegister))
			t.Reset(10 * time.Second) // LWL pairing ends after ~15s
		}
	}
}

// QueryAllRadiators queries the LWL for a list of paired devices, then
// requests the status of each.
func (c *Client) QueryAllRadiators(ctx context.Context) error {
	chr := make(chan Response, 10)
	c.Subscribe("", chr, nil)
	r, err := c.Do(ctx, CmdQueryRadiators)
	if err != nil {
		slog.Error("Failed to query radiators: %w")
	}

	// *!{"trans":14674,"mac":"20:3B:85","time":1767297488,"pkt":"room","fn":"summary","stat0":255,"stat1":7,"stat2":0,"stat3":0,"stat4":0,"stat5":0,"stat6":0,"stat7":0,"stat8":0,"stat9":0}
	rooms := make([]uint8, 0, 80) // LWL upper limit is 80 rooms

	bitfields := [...]uint8{
		r.Stat0, r.Stat1, r.Stat2, r.Stat3, r.Stat4,
		r.Stat5, r.Stat6, r.Stat7, r.Stat8, r.Stat9,
	}

	for stat, bits := range bitfields {
		for bit := uint8(0); bit <= 8; bit++ {
			mask := (uint8(1) << bit)
			if bits&mask != 0 {
				// Room numbers start at 1, so bit0 in stat0 refers to room 1.
				slot := 1 + (uint8(stat) * uint8(8)) + bit
				rooms = append(rooms, slot)
			}
		}
	}

	slog.Info("Room summary", "rooms", &rooms)

	for _, room := range rooms {
		doCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()

		id := fmt.Sprintf("R%d", room)
		cmd := *CmdQueryRadiator.New(id)
		r, err := c.Do(doCtx, cmd)
		if err != nil {
			slog.Warn("Invalid response", "cmd", &cmd, "err", err)
			continue
		}

		slog.Info("Response", "cmd", &cmd, "r", &r)
	}

	return nil
}
