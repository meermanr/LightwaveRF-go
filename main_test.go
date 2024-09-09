package main

import "testing"

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
	response := `0,ERR,2,"Not yet registered. See LightwaveLink"`
}
