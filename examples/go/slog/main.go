package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"sync"
	"time"
)

// SyslogHandler is a slog.Handler that ships RFC 5424 messages to logrecv.
// Standard library only, works on every OS (including Windows).
type SyslogHandler struct {
	mu     *sync.Mutex
	conn   net.Conn
	nl     string // "\n" for TCP framing, "" for UDP
	host   string
	app    string
	level  slog.Leveler
	attrs  []slog.Attr
	prefix string // dotted group path
}

// NewSyslogHandler dials network ("udp" or "tcp") and address, e.g. "127.0.0.1:5514".
func NewSyslogHandler(network, addr, app string, level slog.Leveler) (*SyslogHandler, error) {
	conn, err := net.Dial(network, addr)
	if err != nil {
		return nil, err
	}
	host, _ := os.Hostname()
	if host == "" {
		host = "-"
	}
	nl := ""
	if network == "tcp" {
		nl = "\n"
	}
	return &SyslogHandler{mu: &sync.Mutex{}, conn: conn, nl: nl, host: host, app: app, level: level}, nil
}

func (h *SyslogHandler) Close() error { return h.conn.Close() }

func (h *SyslogHandler) Enabled(_ context.Context, l slog.Level) bool {
	return l >= h.level.Level()
}

func severity(l slog.Level) int {
	switch {
	case l >= slog.LevelError:
		return 3 // error
	case l >= slog.LevelWarn:
		return 4 // warning
	case l >= slog.LevelInfo:
		return 6 // info
	default:
		return 7 // debug
	}
}

func flatten(dst map[string]any, prefix string, a slog.Attr) {
	v := a.Value.Resolve()
	if v.Kind() == slog.KindGroup {
		p := prefix
		if a.Key != "" {
			p += a.Key + "."
		}
		for _, ga := range v.Group() {
			flatten(dst, p, ga)
		}
		return
	}
	if a.Key == "" {
		return
	}
	switch x := v.Any().(type) {
	case error:
		dst[prefix+a.Key] = x.Error()
	default:
		dst[prefix+a.Key] = x
	}
}

func (h *SyslogHandler) Handle(_ context.Context, r slog.Record) error {
	fields := map[string]any{}
	for _, a := range h.attrs {
		flatten(fields, "", a)
	}
	r.Attrs(func(a slog.Attr) bool {
		flatten(fields, h.prefix, a)
		return true
	})
	msg := r.Message
	if len(fields) > 0 {
		if b, err := json.Marshal(fields); err == nil {
			msg += " " + string(b)
		}
	}
	const facilityUser = 1
	line := fmt.Sprintf("<%d>1 %s %s %s %d - - %s%s",
		facilityUser*8+severity(r.Level), r.Time.Format(time.RFC3339Nano), h.host, h.app, os.Getpid(), msg, h.nl)
	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := h.conn.Write([]byte(line))
	return err
}

func (h *SyslogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	c := *h
	for _, a := range attrs {
		if h.prefix != "" {
			a = slog.Group(h.prefix[:len(h.prefix)-1], a)
		}
		c.attrs = append(append([]slog.Attr(nil), c.attrs...), a)
	}
	return &c
}

func (h *SyslogHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	c := *h
	c.prefix += name + "."
	return &c
}

func main() {
	h, err := NewSyslogHandler("udp", "127.0.0.1:5514", "my-go-app", slog.LevelDebug)
	if err != nil {
		panic(err)
	}
	defer h.Close()
	log := slog.New(h)

	log.Debug("cache miss", "key", "user:42")
	log.Info("user logged in", "name", "hadi", slog.Group("req", "ip", "10.0.0.5", "ms", 12))
	log.Warn("disk almost full", "free_gb", 1.2)
	log.With("order", 1234).Error("payment failed", "err", errors.New("card declined"))
}
