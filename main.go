// File: main.go
// Author: Hadi Cahyadi <cumulus13@gmail.com>
// Date: 2026-10-28
// Description: 
// License: MIT

package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

var version = "dev"

var (
	sevName = [8]string{"EMERG", "ALERT", "CRIT", "ERROR", "WARN", "NOTICE", "INFO", "DEBUG"}

	re5424    = regexp.MustCompile(`^<(\d{1,3})>\d+ (\S+) (\S+) (\S+) (\S+) (\S+) (?:-|\[.*?\])\s?(.*)$`)
	re3164    = regexp.MustCompile(`^<(\d{1,3})>([A-Z][a-z]{2} [ \d]\d \d{2}:\d{2}:\d{2}) (\S+) (?:([^\s:\[]+)(?:\[(\d+)\])?:\s?)?(.*)$`)
	rePri     = regexp.MustCompile(`^<(\d{1,3})>(.*)$`)
	reLaravel = regexp.MustCompile(`^(\w+)\.(DEBUG|INFO|NOTICE|WARNING|ERROR|CRITICAL|ALERT|EMERGENCY): ?(.*)$`)
	reExtra   = regexp.MustCompile(`(?:^|\s)[{\[]["'{\[]`)
)

type entry struct {
	sev                 int
	host, app, pid, msg string
}

func clean(s string) string {
	if s == "-" {
		return ""
	}
	return s
}

func parse(line string, from net.IP) entry {
	host := "-"
	if from != nil {
		host = from.String()
	}
	e := entry{sev: 6, host: host, msg: line}
	if m := re5424.FindStringSubmatch(line); m != nil {
		pri, _ := strconv.Atoi(m[1])
		e.sev = pri & 7
		if h := clean(m[3]); h != "" {
			e.host = h
		}
		e.app, e.pid, e.msg = clean(m[4]), clean(m[5]), m[7]
		return e
	}
	if m := re3164.FindStringSubmatch(line); m != nil {
		pri, _ := strconv.Atoi(m[1])
		e.sev = pri & 7
		if h := clean(m[3]); h != "" {
			e.host = h
		}
		e.app, e.pid, e.msg = m[4], m[5], m[6]
		return e
	}
	if m := rePri.FindStringSubmatch(line); m != nil {
		pri, _ := strconv.Atoi(m[1])
		e.sev = pri & 7
		e.msg = m[2]
	}
	return e
}

func parseLevel(s string) (int, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	alias := map[string]int{"emerg": 0, "emergency": 0, "alert": 1, "crit": 2, "critical": 2,
		"err": 3, "error": 3, "warn": 4, "warning": 4, "notice": 5, "info": 6, "debug": 7}
	if v, ok := alias[s]; ok {
		return v, nil
	}
	return 0, fmt.Errorf("invalid -level %q (emerg|alert|crit|error|warn|notice|info|debug)", s)
}

func isCtl(r rune) bool { return (r < 0x20 && r != '\t') || r == 0x7f || (r >= 0x80 && r < 0xa0) }

// sanitize drops control chars so a log line can't inject terminal escapes.
func sanitize(s string) string {
	if strings.IndexFunc(s, isCtl) < 0 {
		return s
	}
	return strings.Map(func(r rune) rune {
		if isCtl(r) {
			return -1
		}
		return r
	}, s)
}

func body(t *Theme, e entry) string {
	msg, prefix := e.msg, ""
	if m := reLaravel.FindStringSubmatch(msg); m != nil {
		prefix = t.channel.Paint("["+m[1]+"]") + " "
		msg = m[3]
	}
	trim := strings.TrimSpace(msg)
	if strings.HasPrefix(trim, "#") || strings.HasPrefix(trim, "[stacktrace]") || strings.HasPrefix(trim, "{main}") {
		return prefix + t.trace.Paint(msg)
	}
	if loc := reExtra.FindStringIndex(msg); loc != nil {
		return prefix + t.text[e.sev].Paint(msg[:loc[0]]) + t.extra.Paint(msg[loc[0]:])
	}
	return prefix + t.text[e.sev].Paint(msg)
}

func render(t *Theme, e entry, now time.Time, tf string) string {
	var sb strings.Builder
	sb.WriteString(t.datetime.Paint(now.Format(tf)))
	sb.WriteByte(' ')
	sb.WriteString(t.level[e.sev].Paint(fmt.Sprintf(" %-6s ", sevName[e.sev])))
	sb.WriteByte(' ')
	sb.WriteString(t.host.Paint(e.host))
	if e.app != "" {
		sb.WriteByte(' ')
		sb.WriteString(t.app.Paint(e.app))
		if e.pid != "" {
			sb.WriteString(t.pid.Paint("[" + e.pid + "]"))
		}
	}
	sb.WriteByte(' ')
	sb.WriteString(body(t, e))
	return sb.String()
}

type receiver struct {
	store   *Store
	minSev  int
	grep    string
	timeFmt string
	file    *os.File
	cancel  context.CancelFunc
	mu      sync.Mutex
}

func logf(format string, a ...any) { fmt.Fprintf(os.Stderr, "logrecv: "+format+"\n", a...) }

func (r *receiver) emit(raw string, from net.IP) {
	raw = strings.TrimRight(raw, "\r\n\x00")
	raw = strings.TrimPrefix(raw, "\ufeff")
	if raw == "" {
		return
	}
	e := parse(sanitize(raw), from)
	if e.sev > r.minSev {
		return
	}
	if r.grep != "" && !strings.Contains(strings.ToLower(raw), r.grep) {
		return
	}
	line := render(r.store.theme(), e, time.Now(), r.timeFmt)

	r.mu.Lock()
	defer r.mu.Unlock()
	if _, err := io.WriteString(os.Stdout, line+"\n"); err != nil {
		r.cancel() // stdout closed (e.g. piped to head)
		return
	}
	if r.file != nil {
		if _, err := io.WriteString(r.file, raw+"\n"); err != nil {
			logf("file write: %v", err)
		}
	}
}

func (r *receiver) handle(data string, from net.IP) {
	for _, l := range strings.Split(data, "\n") {
		r.emit(l, from)
	}
}

func (r *receiver) serveUDP(ctx context.Context, pc net.PacketConn) {
	stop := context.AfterFunc(ctx, func() { pc.Close() })
	defer stop()
	buf := make([]byte, 65535)
	for {
		n, a, err := pc.ReadFrom(buf)
		if err != nil {
			if ctx.Err() == nil && !errors.Is(err, net.ErrClosed) {
				logf("udp: %v", err)
			}
			return
		}
		var ip net.IP
		if ua, ok := a.(*net.UDPAddr); ok {
			ip = ua.IP
		}
		r.handle(string(buf[:n]), ip)
	}
}

func (r *receiver) serveTCP(ctx context.Context, ln net.Listener) {
	stop := context.AfterFunc(ctx, func() { ln.Close() })
	defer stop()
	for {
		c, err := ln.Accept()
		if err != nil {
			if ctx.Err() == nil && !errors.Is(err, net.ErrClosed) {
				logf("tcp: %v", err)
			}
			return
		}
		go func(c net.Conn) {
			defer c.Close()
			defer context.AfterFunc(ctx, func() { c.Close() })()
			var ip net.IP
			if ta, ok := c.RemoteAddr().(*net.TCPAddr); ok {
				ip = ta.IP
			}
			sc := bufio.NewScanner(c)
			sc.Buffer(make([]byte, 64*1024), 1024*1024)
			for sc.Scan() {
				r.emit(sc.Text(), ip)
			}
			if err := sc.Err(); err != nil && ctx.Err() == nil {
				logf("tcp %v: %v", c.RemoteAddr(), err)
			}
		}(c)
	}
}

// ---- commands -------------------------------------------------------------

func newFS(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	return fs
}

func cmdRun(args []string) error {
	fs := newFS("logrecv")
	udp := fs.String("udp", ":5514", "UDP listen address (empty to disable)")
	tcp := fs.String("tcp", "", "TCP listen address (empty to disable)")
	level := fs.String("level", "debug", "minimum level: emerg|alert|crit|error|warn|notice|info|debug")
	g := fs.String("grep", "", "only show lines containing this text (case-insensitive)")
	file := fs.String("file", "", "also append raw lines to this file")
	tf := fs.String("timefmt", "15:04:05.000", "Go time layout for the timestamp")
	cfgFlag := fs.String("config", "", "config file (default: per-OS user config dir, or $LOGRECV_CONFIG)")
	colorFlag := fs.String("color", "auto", "auto|always|never|256")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}
	minSev, err := parseLevel(*level)
	if err != nil {
		return err
	}
	path, err := resolvePath(*cfgFlag)
	if err != nil {
		return err
	}
	mode, err := detectMode(*colorFlag)
	if err != nil {
		return err
	}
	store := &Store{path: path, mode: mode}
	if err := store.load(); err != nil {
		return fmt.Errorf("invalid config (see `logrecv show`):\n%w", err)
	}

	ctx, stopSig := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSig()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	r := &receiver{store: store, minSev: minSev, grep: strings.ToLower(*g), timeFmt: *tf, cancel: cancel}
	if *file != "" {
		f, err := os.OpenFile(*file, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}
		defer f.Close()
		r.file = f
	}

	var pc net.PacketConn
	var ln net.Listener
	if *udp != "" {
		if pc, err = net.ListenPacket("udp", *udp); err != nil {
			return fmt.Errorf("udp: %w", err)
		}
		defer pc.Close()
		logf("UDP listening on %s", pc.LocalAddr())
	}
	if *tcp != "" {
		if ln, err = net.Listen("tcp", *tcp); err != nil {
			return fmt.Errorf("tcp: %w", err)
		}
		defer ln.Close()
		logf("TCP listening on %s", ln.Addr())
	}
	if pc == nil && ln == nil {
		return errors.New("nothing to listen on: both -udp and -tcp are empty")
	}
	logf("config: %s (colors: %s, hot-reload on)", path, mode)

	var wg sync.WaitGroup
	done := ctx.Done()
	wg.Add(1)
	go func() { defer wg.Done(); store.watch(done, logf) }()
	if pc != nil {
		wg.Add(1)
		go func() { defer wg.Done(); r.serveUDP(ctx, pc) }()
	}
	if ln != nil {
		wg.Add(1)
		go func() { defer wg.Done(); r.serveTCP(ctx, ln) }()
	}
	<-done
	wg.Wait()
	return nil
}

func cmdInit(args []string) error {
	fs := newFS("init")
	force := fs.Bool("f", false, "overwrite existing config")
	cfgFlag := fs.String("config", "", "config file path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	path, err := resolvePath(*cfgFlag)
	if err != nil {
		return err
	}
	if _, err := os.Stat(path); err == nil && !*force {
		return fmt.Errorf("%s already exists (use -f to overwrite)", path)
	}
	data, err := json.MarshalIndent(defaultConfig(), "", "  ")
	if err != nil {
		return err
	}
	if err := writeFileAtomic(path, append(data, '\n')); err != nil {
		return err
	}
	fmt.Println(path)
	return nil
}

func cmdPath(args []string) error {
	fs := newFS("path")
	cfgFlag := fs.String("config", "", "config file path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	path, err := resolvePath(*cfgFlag)
	if err != nil {
		return err
	}
	fmt.Println(path)
	return nil
}

func cmdShow(args []string) error {
	fs := newFS("show")
	cfgFlag := fs.String("config", "", "config file path")
	colorFlag := fs.String("color", "auto", "auto|always|never|256")
	jsonOnly := fs.Bool("json", false, "print only the effective config as JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	path, err := resolvePath(*cfgFlag)
	if err != nil {
		return err
	}
	cfg, err := loadConfig(path)
	if err != nil {
		return err
	}
	data, _ := json.MarshalIndent(cfg, "", "  ")
	if *jsonOnly {
		fmt.Println(string(data))
		return nil
	}
	mode, err := detectMode(*colorFlag)
	if err != nil {
		return err
	}
	th, err := compile(cfg, mode)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	note := ""
	if _, err := os.Stat(path); err != nil {
		note = " (not found, showing defaults)"
	}
	fmt.Printf("path:   %s%s\ncolors: %s\n\n%s\n\npreview:\n", path, note, mode, data)
	now := time.Now()
	for i := range sevName {
		e := entry{sev: i, host: "web-01", app: "nginx", pid: "4242", msg: `Request handled {"user":42,"ok":true}`}
		fmt.Println(render(th, e, now, "15:04:05.000"))
	}
	return nil
}

func usage(w io.Writer) {
	fmt.Fprint(w, `logrecv - colorful syslog/GNTP-style log receiver (UDP/TCP)

Usage:
  logrecv [run] [flags]     start receiver (default)
  logrecv init [-f]         write default config file
  logrecv show [-json]      show effective config + color preview
  logrecv path              print config file path
  logrecv version

Run flags:
  -udp ADDR      UDP listen address (default ":5514", empty = off)
  -tcp ADDR      TCP listen address (default off)
  -level LEVEL   minimum level (default debug)
  -grep TEXT     only lines containing TEXT
  -file PATH     also append raw lines to PATH
  -timefmt FMT   Go time layout (default 15:04:05.000)
  -config PATH   config file (default per-OS; or $LOGRECV_CONFIG)
  -color MODE    auto|always|never|256

Config is hot-reloaded while running; invalid edits are ignored with a warning.
`)
}

func main() {
	args := os.Args[1:]
	cmd := "run"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		cmd, args = args[0], args[1:]
	}
	var err error
	switch cmd {
	case "run":
		err = cmdRun(args)
	case "init":
		err = cmdInit(args)
	case "show":
		err = cmdShow(args)
	case "path":
		err = cmdPath(args)
	case "version":
		fmt.Println("logrecv", version)
	case "help":
		usage(os.Stdout)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", cmd)
		usage(os.Stderr)
		os.Exit(2)
	}
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		fmt.Fprintln(os.Stderr, "logrecv:", err)
		os.Exit(1)
	}
}
