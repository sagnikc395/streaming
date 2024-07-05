package sdp

import (
	"net/mail"
	"net/url"
	"os"
	"reflect"
	"testing"
)

func TestReadSession(t *testing.T) {
	var cases = []struct {
		name string
		want Session
		err  bool
	}{
		{
			name: "good.sdp",
			want: Session{
				Name:   "Call to John Smith",
				Origin: Origin{"jdoe", 3724394400, 3724394405, "IN", "IP4", "198.51.100.1"},
				Info:   "SDP Offer #1",
				URI: &url.URL{
					Scheme: "http",
					Host:   "www.jdoe.example.com",
					Path:   "/home.html",
				},
				Email: &mail.Address{"Jane Doe", "jane@jdoe.example.com"},
				Phone: "+16175556011",
			},
		},
		{
			name: "some_optional.sdp",
			want: Session{
				Origin: Origin{"jdoe", 3724394400, 3724394405, "IN", "IP4", "198.51.100.1"},
				Name:   "Call to John Smith",
				Email:  &mail.Address{"Jane Doe", "jane@jdoe.example.com"},
			},
		},
		{
			name: "missing_origin.sdp",
			want: Session{},
			err:  true,
		},
		{
			name: "out_of_order.sdp",
			want: Session{},
			err:  true,
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			f, err := os.Open("testdata/" + tt.name)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			session, err := ReadSession(f)
			if err == nil && tt.err {
				t.Fatal("unexpected nil error")
			} else if err != nil && tt.err {
				return
			} else if err != nil && !tt.err {
				t.Fatalf("read session: %v", err)
			}
			if !reflect.DeepEqual(*session, tt.want) {
				t.Errorf("got %+v\nwant %+v\n", *session, tt.want)
			}
			t.Errorf("TODO still not parsing all fields")
		})
	}
}

func TestBandwidth(t *testing.T) {
	var cases = []struct{
		name string
		s string
		err bool
	}{
		{"conference total", "CT:2048", false},
		{"app specific", "AS:87654321", false},
		{"custom", "69something:12345", false},
		{"missing modifier", ":12345", true},
		{"missing separator" "CT2048", true},
	}
	for _, tt := range cases {
		t.Errorf("TODO ", tt.name)
	}
