// Package main implements a service which communicates with a LightwaveRF Link (LWL) to monitor battery levels of peripherals
package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/MatusOllah/slogcolor"

	"github.com/meermanr/LightwaveRF-go/lwl"
)

var isVerbose = flag.Bool("verbose", false, "Enable display of DEBUG log messages")
var wantDeregister = flag.Bool("unpair", false, "Unpair from LightwaveLink")

// Remove leading whitespace from every line of a string. The amount of
// whitespace is calculated from the first (non-blank) line.
func Dedent(in string) (out string) {
	var indent string
	var lines []string

	// Remove exactly one leading newline, if present
	if len(in) >= 1 && in[0] == '\n' {
		in = in[1:]
	}

	// Determine indent of first line by scanning until first non-blank
FindIndent:
	for i, c := range in {
		switch c {
		case ' ', '\t':
			continue
		default:
			indent = in[:i]
			break FindIndent
		}
	}

	// Strip indent from remaining lines
	for s := range strings.SplitSeq(in, "\n") {
		line := strings.TrimPrefix(s, indent)
		lines = append(lines, line)
	}

	out = strings.Join(lines, "\n")
	return
}

func main() {
	// Command line arguments
	flag.Parse()

	// Logging
	opts := slogcolor.DefaultOptions
	switch *isVerbose {
	case true:
		opts.Level = slog.LevelDebug
	case false:
		opts.Level = slog.LevelInfo
	}
	slog.SetDefault(slog.New(slogcolor.NewHandler(os.Stderr, opts)))
	slog.Debug("Debug messages look like this")

	// LightwaveLink
	c := lwl.New()
	msgs := make(chan lwl.Response, 10)
	sid := c.Subscribe("", msgs, nil)
	defer c.Unsubscribe(sid)
	go c.Listen()

	if *wantDeregister {
		slog.Info("Deregister", "response", c.DoLegacy(lwl.CmdDeregister))
	}

	c.EnsureRegistered()

	// Test connectivity
	// :dcaffe,123,!F*p
	// println("DoLegacy(@H)", c.DoLegacy("@H"))
	// println(c.String())
	// println("DoLegacy(!F*p)", c.DoLegacy("!F*p"))
	// println(c.String())

	slog.Info("Starting main loop")
	t := time.NewTimer(10 * time.Second)
	for {
		select {
		case msg := <-msgs:
			fmt.Printf("%+v\n", msg)
		case <-t.C:
			fmt.Printf("%v\n", c)
		}
	}
}
