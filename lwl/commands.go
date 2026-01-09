package lwl

import (
	"fmt"
	"log/slog"
)

// Command represents a command which can be Sent() to the LWL
type Command struct {
	cmd        string              // Format string of transmitted command
	opts       []any               // Format parameters of transmitted command
	legacyOnly bool                // True if this command does NOT generate a JSON response
	pkt        string              // Expected Response.Pkt
	fn         string              // Expected Response.Fn
	match      func(Response) bool // Custom IsResponse implementation, optional
}

// New returns a Command with parameters.
//
// Parameters are used to render the command string, for example querying the
// status of a specific device takes a device id as a parameter. They are also
// used to match responses, e.g. detecting a status message from a specific
// device.
func (c *Command) New(opts ...any) *Command {
	out := c
	out.opts = opts
	return out
}

// String returns a rendered comand, ready to Send
func (c *Command) String() string {
	return fmt.Sprintf(c.cmd, c.opts...)
}

// LogValue implements slog.LogValuer.
func (c Command) LogValue() slog.Value {
	return slog.StringValue(c.String())
}

// IsResponse checks if a given message is likely to be a response to this command.
//
// For example the command "@H" expects Response.Fn=="hubCall", and a status
// query for a given device expects a status from that specific device.
func (c *Command) IsResponse(r Response) bool {
	switch {
	case c.match != nil:
		return c.match(r)
	case c.fn != "" && c.pkt != "":
		return r.Fn == c.fn && r.Pkt == c.pkt
	case c.fn != "":
		return r.Fn == c.fn
	case c.pkt != "":
		return r.Pkt == c.pkt
	default:
		return false
	}
}

// CmdRegister will pair the current LAN host (identified by MAC address) with
// LWL. If already paired LWL will response with a legacy message containing
// it's version, e.g. "?V=\"N2.94D\""
//
//	->: 3,!F*p
//
// Replies if unpaired:
//
//	<-: 3,ERR,2,"Not yet registered. See LightwaveLink"\r\n
//	<-: *!{"trans":19657,"mac":"20:3B:85","time":1767795432,"pkt":"error","fn":"nonRegistered","payload":"Not yet registered. See LightwaveLink"}
//
// LWL then enters pairing mode, where it flashes its LED to prompt the user to push it. Upon being pushed, it will response:
//
//	<-: *!{"trans":19659,"mac":"20:3B:85","time":1767795442,"type":"link","prod":"lwl","pairType":"local","msg":"success","class":"","serial":""}
//
// If already paired:
//
//	<-: 3,?V="N2.94D"\r\n
var CmdRegister = Command{cmd: "!F*p", legacyOnly: true}

// CmdDeregister will unpair the current LAN host from LWL (only works when
// already paired)
//
//	->: 2,!F*xP
//	<-: 2,OK\n
var CmdDeregister = Command{cmd: "!F*xP", legacyOnly: true}

// CmdHubCall find out information from the Link unit to help understand its
// behaviour (number of energy and heating devices, etc)
//
//	->: 3,@H
//	<-: *!{"trans":19686,"mac":"20:3B:85","time":1767795878,"pkt":"system","fn":"hubCall","type":"hub","prod":"lwl","fw":"N2.94D","uptime":3300881,"timeZone":0,"lat":52.18,"long":0.21,"tmrs":1,"evns":5,"run":0,"macs":1,"ip":"192.168.4.71","devs":11}
//	<-: 3,OK\n
var CmdHubCall = Command{cmd: "@H", fn: "hubCall"}

// CmdHubDuskDawn finds when dusk and dawn time values used by timers
//
//	->: 3,@D
//	<-: *!{"trans":19994,"mac":"20:3B:85","time":1767824683,"pkt":"duskDawn","fn":"read","duskTime":1767801880,"dawnTime":1767773171}
//	<-: 3,OK\n
var CmdHubDuskDawn = Command{cmd: "@D", pkt: "duskDawn"}

// CmdSetTimezone sets the GMT offset in integer hours. Note that the Link will
// automatically change to DST; you do not need to account for this when
// setting this up. i.e. If you are in BST, when you set the timezone, you
// would send !FzP0 and not !FzP1. Args:
//
//   - int  GMT offset, in hours. Can be positive of negative.
var CmdSetTimezone = Command{cmd: "!FzP%d"}

// CmdSetLocation sets the latitude and longtitude of the LWL. Used to
// determine dawn and dusk times. Args:
//
//   - float  Latitude, e.g. 52.1837667
//   - float  Longtide, e.g. 0.2078069
var CmdSetLocation = Command{cmd: "!FqP\"%f,%f\""}

// CmdSetHubUIBright sets the LED on the Link on, and on the LW500, brighten the screen
//
// ->: 3,@L1
// <-: 3,OK\n
var CmdSetHubUIBright = Command{cmd: "@L1", legacyOnly: true}

// CmdSetHubUIDim sets the LED on the Link off, and on the LW500, dim the screen
//
// ->: 3,@L0
// <-: 3,OK\n
var CmdSetHubUIDim = Command{cmd: "@L0", legacyOnly: true}

// CmdOn turns on a device. Args:
//
//   - string  Room+Device identifier, e.g. R1D1
var CmdOn = Command{cmd: "!%sF1"}

// CmdOff turns off a device. Args:
//
//   - string  Room+Device identifier, e.g. R1D1
var CmdOff = Command{cmd: "!%sF0"}

// CmdSetDimmer sets the brightness of a dimmer. Args:
//
//   - string  Room+Device identifier, e.g. R1D1
//     int     Brightness level, 1-32 (inc.). 1=Dimmest, 32=Brightest
var CmdSetDimmer = Command{cmd: "!%sFdP%d"}

// CmdOpen Opens a relay (no connection). Args:
//
//   - string  Room+Device identifier, e.g. R1D1
var CmdOpen = Command{cmd: "!%sF("}

// CmdClose Closes a relay (make a connection). Args:
//
//   - string  Room+Device identifier, e.g. R1D1
var CmdClose = Command{cmd: "!%sF)"}

// CmdStop Stops a relay (?). Args:
//
//   - string  Room+Device identifier, e.g. R1D1
var CmdStop = Command{cmd: "!%sF^"}

// CmdLEDColourSet sets the colour of an LED colour changing product. Args:
//
//   - string  Room+Device identifier, e.g. R1D1
//   - int32   Colour, see below.
//
// Where colour is one of:
//
//   - 1  White
//   - 2  Green White
//   - 3  Red White
//   - 4  Yellow White
//   - 5  Red
//   - 6  Middle Red
//   - 7  Pink
//   - 8  Light Red
//   - 9  Orange
//   - 10 Middle Orange
//   - 11 Green White
//   - 12 Light Orange
//   - 13 Green
//   - 14 Middle Green
//   - 15 Light Yellow Green
//   - 16 Yellow Green
//   - 17 Blue
//   - 18 Water Blue
//   - 19 Light Blue
//   - 20 White Blue
var CmdLEDColourSet = Command{cmd: "!%sF*cP%d"}

// CmdLEDColourCycle progresses a colour changing product to the next cycling
// mode. Args:
//
//   - string  Room+Device identifier, e.g. R1D1
//
// Colour changing products have four cycle modes:
//
//   - Between current colour pallette (White, Red, Orange, Green, Blue) - Fast
//   - Between current colour pallette (White, Red, Orange, Green, Blue) - Slow
//   - Between all colour pallettes - Fast
//   - Between all colour pallettes - Slow
var CmdLEDColourCycle = Command{cmd: "!%sF*y"}

// CmdLockPartial configures a device so it cannot be switched manually but can
// be from an RF device (such as the WiFiLink, remote control, PIR etc). Args:
//
//   - string  Room+Device identifier, e.g. R1D1
var CmdLockPartial = Command{cmd: "!%sFl"}

// CmdLockFull configures a device so it cannot be switched either manually nor
// from an RF command until unlocked. Args:
//
//   - string  Room+Device identifier, e.g. R1D1
var CmdLockFull = Command{cmd: "!%sFk"}

// CmdUnlock configures a device that was previously locked to accept control
// from all inputs. Args:
//
//   - string  Room+Device identifier, e.g. R1D1
var CmdUnlock = Command{cmd: "!%sFu"}

// CmdMoodStore will store the current status of all devices in a given room to
// a given mood slot. Args:
//
//   - string  Room identifier, e.g. R1
//   - int32   Mood slot, 1-5 (inc.)
//
// Note that some moods have conventional use:
//
//   - 4 Entry
//   - 5 Exit
var CmdMoodStore = Command{cmd: "!%sFsP%d"}

// CmdMoodRecall will set the status of all deviced in a given room to a mood
// which was saved previously. Args:
//
//   - string  Room identifier, e.g. R1
//   - int32   Mood slot, 1-5 (inc.)
//
// Note that some moods have conventional use:
//
//   - 4 Entry
//   - 5 Exit
var CmdMoodRecall = Command{cmd: "!%sFmP%d"}

// CmdAllOff turns off all devices in a given room. Args:
//
//   - string  Room identifier, e.g. R1
var CmdAllOff = Command{cmd: "!%sFa"}

// CmdPairDevice places the hub into Linking mode, ready for a heating or
// energy device to register with it. The user specified the Room number to
// assign to the registering device. Args:
//
//   - string  Room identifier, e.g. R1
var CmdPairDevice = Command{cmd: "!%sF*L"}

// CmdUnpairDevice instructs the hub to forget a paired device.
//
//   - string  Room identifier, e.g. R1
var CmdUnpairDevice = Command{cmd: "!%sF*xU"}

// CmdQueryRadiators finds which radiator ("room") numbers have been allocated.
//
//	->: 5,@R
//	<-: *!{"trans":20021,"mac":"20:3B:85","time":1767830010,"pkt":"room","fn":"summary","stat0":255,"stat1":7,"stat2"90 "stat3":0,"stat4":0,"stat5":0,"stat6":0,"stat7":0,"stat8":0,"stat9":0}
//	<-: 5,OK\n
var CmdQueryRadiators = Command{cmd: "@R", pkt: "room", fn: "summary"}

// CmdQueryRadiator instructs a specific radiator to report its product
// information. Args:
//
//   - string  Room identifier, e.g. R1
//
// Sample data:
//
//	->: 13,@?R8
//	<-: *!{"trans":20073,"mac":"20:3B:85","time":1767831552,"pkt":"room","fn":"read","slot":8,"serial":"6E8002","prod":"valve"}
//	<-: 13,OK\n
var CmdQueryRadiator = Command{cmd: "@?%s", pkt: "room", fn: "read"}
