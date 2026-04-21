package lwl

import (
	"reflect"
	"testing"
)

// FIXME: Test now fails, as expected. Need to change Response.Payload -> any
// type, and then work out how to practically live with any types (custom
// decoder from json.RawMessage?)
func TestPayload(t *testing.T) {
	table := []struct {
		n string       // name of the test
		j string       // JSON string data
		t reflect.Type // Expected type
	}{
		{
			n: `string`,
			j: `*!{"trans":12090,"mac":"20:3B:85","time":1766967067,"pkt":"error","fn":"nonRegistered","payload":"Not yet registered. See LightwaveLink"}`,
			t: reflect.TypeFor[string](),
		},
		{
			n: `absent`,
			j: `*!{"trans":13367,"mac":"20:3B:85","time":1767129960,"type":"link","prod":"lwl","pairType":"local","msg":"success","class":"","serial":""}`,
			t: nil,
		},
		{
			n: `number`,
			j: `*!{"trans":93150,"mac":"20:3B:85","time":1776726215,"pkt":"868R","fn":"ack","status":"success","attempts":1,"packet":208,"type":"log","payload":208}`,
			t: reflect.TypeFor[float64](),
		},
	}

	c := Client{}
	for _, test := range table {
		t.Run(test.n, func(t *testing.T) {
			r, err := c.parseJSON(string(test.j))
			if err != nil {
				t.Fatal(err)
			}
			if reflect.TypeOf(r.Payload) != test.t {
				t.Fatalf("r.Payload wrong type. want %s got %s", test.t, reflect.TypeOf(r.Payload))
			}
		})
	}
	//	*!{"trans":12090,"mac":"20:3B:85","time":1766967067,"pkt":"error","fn":"nonRegistered","payload":"Not yet registered. See LightwaveLink"}
	//	*!{"trans":13367,"mac":"20:3B:85","time":1767129960,"type":"link","prod":"lwl","pairType":"local","msg":"success","class":"","serial":""}
	//	*!{"trans":14619,"mac":"20:3B:85","time":1767288212,"pkt":"system","fn":"hubCall","type":"hub","prod":"lwl","fw":"N2.94D","uptime":2790197,"timeZone":0,"lat":52.18,"long":0.21,"tmrs":1,"evns":5,"run":0,"macs":1,"ip":"192.168.4.71","devs":11}
	//	*!{"trans":14674,"mac":"20:3B:85","time":1767297488,"pkt":"room","fn":"summary","stat0":255,"stat1":7,"stat2":0,"stat3":0,"stat4":0,"stat5":0,"stat6":0,"stat7":0,"stat8":0,"stat9":0}
	//	*!{"trans":14819,"mac":"20:3B:85","time":1767307528,"pkt":"room","fn":"read","slot":10,"serial":"D88002","prod":"valve"}
	//	*!{"trans":93136,"mac":"20:3B:85","time":1776726001,"pkt":"868R","fn":"statusPush","prod":"valve","serial":"24C702","type":"temp","batt":3.03,"ver":58,"state":"run","cTemp":19.4,"cTarg":19.0,"output":0,"nTarg":17.0,"nSlot":"00:00","prof":1}
	//
	// FIXME: Payload is sometimes a number, othertimes a string!
	//
	//	*!{"trans":93150,"mac":"20:3B:85","time":1776726215,"pkt":"868R","fn":"ack","status":"success","attempts":1,"packet":208,"type":"log","payload":208}

}
