package main

import (
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
// will change this to the found unicast address.
var lwl_ip_address = "255.255.255.255"
var lwl_mac_address = "" // To be discovered at runtime
var sid = 0              // Sequence ID

var state_tmpl *template.Template

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
		switch {
		case c == ' ', c == '\t':
			continue
		default:
			indent = in[:i]
			break FindIndent
		}
	}

	// Strip indent from remaining lines
	for _, s := range strings.Split(in, "\n") {
		line := strings.TrimPrefix(s, indent)
		lines = append(lines, line)
	}

	out = strings.Join(lines, "\n")
	return
}

func init() {
	var err error

	state_tmpl, err = template.New("test").Parse(Dedent(`
		State
		-----

		Lwl_server_port = {{.Lwl_server_port}}
		Lwl_client_port = {{.Lwl_client_port}}
		Lwl_ip_address = {{.Lwl_ip_address}}

		`))
	if err != nil {
		panic(err)
	}
}

func main() {
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

	var inq chan string = make(chan string, 10)
	go func() {
		con, err := net.ListenUDP("udp", &net.UDPAddr{Port: lwl_client_port})
		if err != nil {
			panic(err)
		}
		var msg []byte = make([]byte, 4096)
		for {
			i, err := con.Read(msg)
			// i, err := con.Read(msg)
			if err != nil {
				panic(err)
			}
			inq <- string(msg[:i])
		}
	}()

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

	// time.Sleep(time.Second * 3)

	ans := <-inq
	fmt.Printf("Answer: %v", ans)
}
