package core

import "testing"

func TestParseCoreWSURL(t *testing.T) {
	host, port, err := parseCoreWSURL("ws://127.0.0.1:8765/ws")
	if err != nil || host != "127.0.0.1" || port != 8765 {
		t.Fatalf("got host=%q port=%d err=%v", host, port, err)
	}
	if host, port, err = parseCoreWSURL(" ws://0.0.0.0:9000/ws "); err != nil || host != "0.0.0.0" || port != 9000 {
		t.Fatalf("whitespace: got host=%q port=%d err=%v", host, port, err)
	}
	for _, raw := range []string{
		"",
		"http://127.0.0.1:8765/ws",
		"wss://127.0.0.1:8765/ws",
		"ws://127.0.0.1/ws",
		"ws://:8765/ws",
		"ws://127.0.0.1:0/ws",
		"ws://127.0.0.1:70000/ws",
		"ws://127.0.0.1:notaport/ws",
	} {
		if _, _, err := parseCoreWSURL(raw); err == nil {
			t.Errorf("parseCoreWSURL(%q) accepted an invalid URL", raw)
		}
	}
}
