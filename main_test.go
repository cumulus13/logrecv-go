// File: main_test.go
// Author: Hadi Cahyadi <cumulus13@gmail.com>
// Date: 2026-10-28
// Description:
// License: MIT

package main

import (
	"bufio"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseHex(t *testing.T) {
	ok := map[string]rgb{
		"#00AAFF": {0, 170, 255}, "00aaff": {0, 170, 255},
		"##FFFFFF": {255, 255, 255}, "#fa0": {255, 170, 0},
	}
	for in, want := range ok {
		got, err := parseHex(in)
		if err != nil || got != want {
			t.Errorf("parseHex(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	for _, in := range []string{"", "#12", "#GGGGGG", "#1234567", "+12345"} {
		if _, err := parseHex(in); err == nil {
			t.Errorf("parseHex(%q): expected error", in)
		}
	}
}

func TestStyleModes(t *testing.T) {
	s, err := newStyle(modeTrue, "#112233", "#445566")
	if err != nil {
		t.Fatal(err)
	}
	if got := s.Paint("x"); got != "\x1b[38;2;17;34;51m\x1b[48;2;68;85;102mx"+reset {
		t.Errorf("truecolor paint = %q", got)
	}
	s, _ = newStyle(mode256, "#FFFFFF", "")
	if got := s.Paint("x"); got != "\x1b[38;5;231mx"+reset {
		t.Errorf("256 paint = %q", got)
	}
	s, err = newStyle(modeNone, "#FFFFFF", "")
	if err != nil || s.Paint("x") != "x" {
		t.Errorf("none mode must not style: %q %v", s.Paint("x"), err)
	}
	if _, err := newStyle(modeNone, "bogus", ""); err == nil {
		t.Error("invalid hex must error even in modeNone")
	}
}

func TestParse(t *testing.T) {
	ip := net.ParseIP("10.0.0.1")
	e := parse(`<11>1 2026-10-28T10:00:00Z web01 nginx 42 - - upstream failed`, ip)
	if e.sev != 3 || e.host != "web01" || e.app != "nginx" || e.pid != "42" || e.msg != "upstream failed" {
		t.Errorf("5424: %+v", e)
	}
	e = parse(`<30>Oct 28 10:00:00 db01 postgres[7]: ready`, ip)
	if e.sev != 6 || e.host != "db01" || e.app != "postgres" || e.pid != "7" || e.msg != "ready" {
		t.Errorf("3164: %+v", e)
	}
	e = parse(`plain text`, nil)
	if e.sev != 6 || e.host != "-" {
		t.Errorf("plain: %+v", e)
	}
}

func TestSanitize(t *testing.T) {
	if got := sanitize("a\x1b[2Jb\tc\x00"); got != "a[2Jb\tc" {
		t.Errorf("sanitize = %q", got)
	}
}

func TestRenderExtraAndTrace(t *testing.T) {
	th, err := compile(defaultConfig(), modeTrue)
	if err != nil {
		t.Fatal(err)
	}
	out := render(th, entry{sev: 6, host: "h", msg: `ok {"a":1}`}, time.Unix(0, 0), "15:04")
	if !strings.Contains(out, "\x1b[38;2;0;255;0mok\x1b[0m") || !strings.Contains(out, "\x1b[38;2;255;255;255m {\"a\":1}") {
		t.Errorf("extra not split/colored: %q", out)
	}
	out = render(th, entry{sev: 3, host: "h", msg: "#0 /app/x.php(1)"}, time.Unix(0, 0), "15:04")
	if !strings.Contains(out, "\x1b[38;2;255;255;255m#0 /app/x.php(1)") {
		t.Errorf("trace not colored: %q", out)
	}
}

func TestConfigLoadMerge(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "c.json")

	if cfg, err := loadConfig(p); err != nil || cfg.Datetime != "#00AAFF" {
		t.Fatalf("missing file must give defaults: %v %v", cfg.Datetime, err)
	}
	write := func(s string) {
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("\xef\xbb\xbf" + `{"datetime":"#111111","levels":{"info":{"text":{"fg":"#222222"}}}}`)
	cfg, err := loadConfig(p)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Datetime != "#111111" || cfg.Levels.Info.Text.FG != "#222222" || cfg.Levels.Info.Text.BG != "" {
		t.Errorf("override failed: %+v", cfg)
	}
	if cfg.Levels.Info.Level.BG != "#00FF00" || cfg.App != "#00AA00" {
		t.Errorf("defaults lost: %+v", cfg)
	}
	write(`{"bogus":1}`)
	if _, err := loadConfig(p); err == nil {
		t.Error("unknown field must error")
	}
	write("")
	if _, err := loadConfig(p); err != nil {
		t.Errorf("empty file must be ok: %v", err)
	}
	write(`{"pid":"nope"}`)
	cfg, _ = loadConfig(p)
	if _, err := compile(cfg, modeNone); err == nil {
		t.Error("bad hex must fail compile")
	}
}

func TestStoreReload(t *testing.T) {
	p := filepath.Join(t.TempDir(), "c.json")
	s := &Store{path: p, mode: modeTrue}
	if err := s.load(); err != nil {
		t.Fatal(err)
	}
	before := s.theme()
	if err := os.WriteFile(p, []byte(`{"datetime":"#010203"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := s.load(); err != nil {
		t.Fatal(err)
	}
	if s.theme() == before || s.theme().datetime.pre != "\x1b[38;2;1;2;3m" {
		t.Error("theme not swapped")
	}
	if err := os.WriteFile(p, []byte(`{"datetime":"zzz!"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	cur := s.theme()
	if err := s.load(); err == nil || s.theme() != cur {
		t.Error("bad config must not replace theme")
	}
}

func TestParseLevel(t *testing.T) {
	for in, want := range map[string]int{"emerg": 0, "ERR": 3, "warning": 4, "debug": 7} {
		if got, err := parseLevel(in); err != nil || got != want {
			t.Errorf("parseLevel(%q) = %d, %v", in, got, err)
		}
	}
	if _, err := parseLevel("loud"); err == nil {
		t.Error("expected error")
	}
}

func TestParseGoSyslogFormat(t *testing.T) {
	// log/syslog: <PRI>RFC3339 host tag[pid]: msg
	e := parse(`<11>2026-10-28T10:00:00+07:00 web01 myapp[77]: boom`, net.ParseIP("10.0.0.1"))
	if !e.pri || e.sev != 3 || e.host != "web01" || e.app != "myapp" || e.pid != "77" || e.msg != "boom" {
		t.Errorf("rfc3339 3164: %+v", e)
	}
}

func TestInheritContinuation(t *testing.T) {
	first := parse(`<11>Oct 28 10:00:00 vm myapp[9]: boom`, nil)
	cont := parse(`Traceback (most recent call last):`, net.ParseIP("1.2.3.4"))
	inherit(&cont, &first)
	if cont.sev != 3 || cont.host != "vm" || cont.app != "myapp" || cont.pid != "9" {
		t.Errorf("continuation did not inherit: %+v", cont)
	}
	own := parse(`<14>other line`, nil)
	inherit(&own, &first)
	if own.sev != 6 || own.app != "" {
		t.Errorf("line with own PRI must not inherit: %+v", own)
	}
	lone := parse(`plain`, nil)
	inherit(&lone, nil)
	if lone.sev != 6 {
		t.Errorf("no previous line: %+v", lone)
	}
}

func TestParseSpaceAfterPRI(t *testing.T) {
	// some senders put a space between <PRI> and the timestamp
	e := parse(`<11> Oct 28 11:06:29 LIC-X TESTME: TEST 001`, net.ParseIP("127.0.0.1"))
	if !e.pri || e.sev != 3 || e.host != "LIC-X" || e.app != "TESTME" || e.msg != "TEST 001" {
		t.Errorf("space + tag: %+v", e)
	}
	e = parse(`<11> Oct 28 11:06:02 LIC-X TEST 001"`, net.ParseIP("127.0.0.1"))
	if e.sev != 3 || e.host != "LIC-X" || e.app != "" || e.msg != `TEST 001"` {
		t.Errorf("space, no tag: %+v", e)
	}
	e = parse(`<15> 1 2026-10-28T10:00:00Z h a 1 - - hi`, nil)
	if e.sev != 7 || e.host != "h" || e.app != "a" || e.msg != "hi" {
		t.Errorf("space + 5424: %+v", e)
	}
	e = parse(`<15>   plain`, nil)
	if e.sev != 7 || e.msg != "plain" {
		t.Errorf("space + bare PRI: %+v", e)
	}
}

func TestParseNoHostname(t *testing.T) {
	ip := net.ParseIP("10.1.1.1")
	e := parse(`<14>Oct 28 04:29:59 my-rust-app[741]: user logged in`, ip)
	if e.sev != 6 || e.host != "10.1.1.1" || e.app != "my-rust-app" || e.pid != "741" || e.msg != "user logged in" {
		t.Errorf("no hostname: %+v", e)
	}
	// the normal "host tag: msg" form must be unaffected
	e = parse(`<14>Oct 28 04:29:59 vm myapp[9]: hi`, ip)
	if e.host != "vm" || e.app != "myapp" || e.pid != "9" || e.msg != "hi" {
		t.Errorf("with hostname: %+v", e)
	}
	e = parse(`<11> Oct 28 11:06:29 LIC-X TESTME: TEST 001`, ip)
	if e.host != "LIC-X" || e.app != "TESTME" {
		t.Errorf("space + hostname + tag: %+v", e)
	}
}

func TestSplitLines(t *testing.T) {
	sc := bufio.NewScanner(strings.NewReader("one\ntwo\x00three\r\nlast"))
	sc.Split(splitLines)
	var got []string
	for sc.Scan() {
		got = append(got, strings.TrimRight(sc.Text(), "\r"))
	}
	want := []string{"one", "two", "three", "last"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("splitLines = %q, want %q", got, want)
	}
}
