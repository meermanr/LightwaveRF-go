// Package main implements a service which communicates with a LightwaveRF Link (LWL) to monitor battery levels of peripherals
package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/meermanr/LightwaveRF-go/lwl"
)

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
	c := lwl.New()
	msgs := make(chan lwl.Response, 10)
	go c.Listen(msgs)

	// Uncomment to deregister, useful during development
	// c.Send("!F*xP", nil, nil)
	c.EnsureRegistered()

	// Test connectivity
	// :dcaffe,123,!F*p
	// println("DoLegacy(@H)", c.DoLegacy("@H"))
	// println(c.String())
	// println("DoLegacy(!F*p)", c.DoLegacy("!F*p"))
	// println(c.String())

	println("Starting main loop")
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
