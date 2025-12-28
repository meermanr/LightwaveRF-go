package main

import (
	"testing"

	"github.com/meermanr/LightwaveRF-go/lwl"
)

func TestDedent(t *testing.T) {
	type args struct {
		in string
	}
	tests := []struct {
		name    string
		args    args
		wantOut string
	}{
		{
			name:    "Empty",
			args:    args{``},
			wantOut: ``,
		},
		{
			name: "SingleBlank",
			args: args{`
`},
			wantOut: ``,
		},
		{
			name: "TwoBlank",
			args: args{`

`},
			wantOut: `
`,
		},
		{
			name: "ThreeBlank",
			args: args{`


`},
			wantOut: `

`,
		},
		{
			name:    "OneFlat",
			args:    args{`No indent here`},
			wantOut: `No indent here`,
		},
		{
			name: "TwoFlat",
			args: args{`
No indent here`},
			wantOut: `No indent here`,
		},
		{
			name: "SingleIndent",
			args: args{`
	No indent here`},
			wantOut: `No indent here`,
		},
		{
			name: "SingleIndentB",
			args: args{`
		No indent here`},
			wantOut: `No indent here`,
		},
		{
			name: "TwoIndentA",
			args: args{`
		Some indent
		
		Another line`},
			wantOut: `Some indent

Another line`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if gotOut := Dedent(tt.args.in); gotOut != tt.wantOut {
				t.Errorf("Dedent() = %v, want %v", gotOut, tt.wantOut)
			}
		})
	}
}

func TestIsRegistered(t *testing.T) {
	// TODO: Work out how to mock messages (... use a channel?)
	response := `*!{"trans":10064,"mac":"20:3B:85","time":1766691793,"pkt":"error","fn":"nonRegistered","payload":"Not yet registered. See LightwaveLink"}`
	println("Logging response to keep the compiler quiet", response)

	lwl := lwl.New()
	println(lwl)
	t.Log("Logging test")

}
