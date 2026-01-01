package lwl

import "fmt"

// Command represents a command which can be Sent() to the LWL
type Command struct {
	cmd string
}

// String returns a rendered comand, ready to Send
func (c *Command) String(opts ...any) string {
	return fmt.Sprintf(c.cmd, opts...)
}

// CmdRegister will pair the current LAN host (identified by MAC address) with
// LWL. If already paired LWL will response with a legacy message containing
// it's version, e.g. "?V=\"N2.94D\""
var CmdRegister = Command{cmd: "!F*p"}

// CmdDeregister will unpair the current LAN host from LWL (only works when
// already paired)
var CmdDeregister = Command{cmd: "!F*xP"}

// CmdHubCall find out information from the Link unit to help understand its
// behaviour (number of energy and heating devices, etc)
var CmdHubCall = Command{cmd: "@H"}

// CmdHubDuskDawn finds when dusk and dawn time values used by timers
var CmdHubDuskDawn = Command{cmd: "@D"}

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
var CmdSetHubUIBright = Command{cmd: "@L1"}

// CmdSetHubUIDim sets the LED on the Link off, and on the LW500, dim the screen
var CmdSetHubUIDim = Command{cmd: "@L0"}

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
var CmdQueryRadiators = Command{cmd: "@R"}

// CmdQueryRadiator instructs a specific radiator to report its product
// information. Args:
//
//   - string  Room identifier, e.g. R1
var CmdQueryRadiator = Command{cmd: "@?%s"}
