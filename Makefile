# Monitor LAN for LWL traffic, showing the source and payload, e.g.
#   Capturing on 'eth0'
#   192.168.4.200   1,@H
#   192.168.4.71    *!{"trans":13149,"mac":"20:3B:85","time":1767106420,"pkt":"error","fn":"nonRegistered","payload":"Not yet registered. Send !F*p to register"}
monitor:
	tshark --color -f 'port 9760 or port 9761' -Tfields -e ip.src -e data.text -o data.show_as_text:TRUE

# Extract JSON messages from log
json:
	@sed -ne 's/^.*\*!\({.*}$$\)/\1/p' lwl.log

# Run tests
test:
	go test --shuffle=on ./...
