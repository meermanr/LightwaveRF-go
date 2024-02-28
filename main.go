package main

import (
	"bytes"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"text/template"
)

const lwl_server_port = 9760 // We send to this address ...
const lwl_client_port = 9761 // ... and listen for responses on this one

// Initially we will use broadcasts to find the Lightwave Link. Once found we
// will change this to the found the unicast address.
var lwl_ip_address = "255.255.255.255"
var lwl_mac_address string = "" // To be discovered at runtime
var sid = 0                     // Sequence ID

var state_tmpl *template.Template

// Dedent removes indentation from a multi-line string.
// The indentation level is determined automatically by counting the number of
// whitespace characters on the first non-empty line.
//
// If the first line is empty, it is not emitted. If the last line is empty, it
// is not emitted. This is convenient when using string literals, e.g.:
//
// Dedent(`
//
//	Golang is the best
//	Who needs generics or maps?
//	Just use interfaces
//	`)
func Dedent(in string) string {
	var indent string
	var line string
	var remaining string = in
	var buf bytes.Buffer
	var found bool

	// Special case: Remove first character if it is a newline
	if len(in) > 1 && in[0] == '\n' {
		in = in[1:]
	}

IndentScanner:
	for {
		// Get next line
		line, remaining, _ = strings.Cut(remaining, "\n")

		// Skip empty lines
		if line == "" {
			continue
		}

		// Scan this non-empty for leading whitespace
		for i := range len(line) {
			c := line[i]
			switch c {
			case ' ', '\t':
				continue
			default:
				// First rune that isn't whitespace
				indent = line[:i]
				break IndentScanner
			}
		}
	}
	// fmt.Printf("Indent is %#v, length %d\n", indent, len(indent))
	// Make a copy of in without indentation
	remaining = in
	for remaining != "" {
		line, remaining, found = strings.Cut(remaining, "\n")
		line, _ = strings.CutPrefix(line, indent)
		// fmt.Printf("line: %#v, found: %v, remaining: %#v\n", line, found, remaining)
		if _, err := buf.WriteString(line); err != nil {
			panic(err)
		}
		// Special case: Omit last line if it is empty (after stripping indent)
		// i.e. do not process the final line
		if found && remaining != indent {
			if _, err := buf.WriteRune('\n'); err != nil {
				panic(err)
			}
		}
	}
	return buf.String()
}

func init() {
	var err error

	state_tmpl, err = template.New("test").Parse(Dedent(`
		Constants
		---------

		Lwl_server_port = {{.Lwl_server_port}}
		Lwl_client_port = {{.Lwl_client_port}}
		Lwl_ip_address = {{.Lwl_ip_address}}

		`))
	if err != nil {
		panic(err)
	}
}

func main() {
	println("Hello, version control archeaologist!.")
	os.Stdout.Sync()
	data := struct {
		Lwl_server_port int
		Lwl_client_port int
		Lwl_ip_address  string
	}{
		Lwl_server_port: lwl_server_port,
		Lwl_client_port: lwl_client_port,
		Lwl_ip_address:  lwl_ip_address,
	}
	err := state_tmpl.Execute(os.Stdout, data)
	if err != nil {
		panic(err)
	}

	// Test connectivity
	// :dcaffe,123,!F*p
	msg := make([]string, 0, 3)
	if lwl_mac_address != "" {
		msg = append(msg, ":"+lwl_mac_address)
	}
	msg = append(msg, fmt.Sprintf("%d", sid))
	sid++
	msg = append(msg, "!F*p")
	var m string = strings.Join(msg, ",")
	fmt.Printf("msg: %#v\n", msg)
	fmt.Printf("m: %s\n", m)

	conn, err := net.Dial("udp", net.JoinHostPort(lwl_ip_address, strconv.Itoa(lwl_server_port)))
	if err != nil {
		panic(err)
	}
	conn.Write([]byte(m))
	conn.Close()
}
