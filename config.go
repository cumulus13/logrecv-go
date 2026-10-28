// File: config.go
// Author: Hadi Cahyadi <cumulus13@gmail.com>
// Date: 2026-10-28
// Description:
// License: MIT

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"
)

// StyleSpec is a foreground/background pair in hex (#RRGGBB or #RGB).
type StyleSpec struct {
	FG string `json:"fg,omitempty"`
	BG string `json:"bg,omitempty"`
}

type LevelSpec struct {
	Level *StyleSpec `json:"level,omitempty"` // the badge
	Text  *StyleSpec `json:"text,omitempty"`  // the message
}

type Levels struct {
	Emergency LevelSpec `json:"emergency"`
	Alert     LevelSpec `json:"alert"`
	Critical  LevelSpec `json:"critical"`
	Error     LevelSpec `json:"error"`
	Warning   LevelSpec `json:"warning"`
	Notice    LevelSpec `json:"notice"`
	Info      LevelSpec `json:"info"`
	Debug     LevelSpec `json:"debug"`
}

// at returns levels indexed by syslog severity (0=emergency .. 7=debug).
func (l *Levels) at() [8]*LevelSpec {
	return [8]*LevelSpec{&l.Emergency, &l.Alert, &l.Critical, &l.Error, &l.Warning, &l.Notice, &l.Info, &l.Debug}
}

var levelKeys = [8]string{"emergency", "alert", "critical", "error", "warning", "notice", "info", "debug"}

type Config struct {
	Datetime string `json:"datetime"`
	Host     string `json:"host"`
	App      string `json:"app"`
	PID      string `json:"pid"`
	Channel  string `json:"channel"` // Laravel-style "[local]" prefix
	Extra    string `json:"extra"`   // trailing json/dict payload
	Trace    string `json:"trace"`   // stack-trace lines
	Levels   Levels `json:"levels"`
}

func defaultConfig() Config {
	sp := func(fg, bg string) *StyleSpec { return &StyleSpec{FG: fg, BG: bg} }
	return Config{
		Datetime: "#00AAFF",
		Host:     "#FFFFFF",
		App:      "#00AA00",
		PID:      "#E8E8E8",
		Channel:  "#FFFFFF",
		Extra:    "#FFFFFF",
		Trace:    "#FFFFFF",
		Levels: Levels{
			Emergency: LevelSpec{sp("#FFFFFF", "#AA00FF"), sp("#FFFFFF", "#AA00FF")},
			Alert:     LevelSpec{sp("#FFFFFF", "#0000FF"), sp("#FFFFFF", "#0000FF")},
			Critical:  LevelSpec{sp("#FFFFFF", "#005500"), sp("#FFFFFF", "#005500")},
			Error:     LevelSpec{sp("#FFFFFF", "#FF0000"), sp("#FFFFFF", "#FF0000")},
			Warning:   LevelSpec{sp("#000000", "#FFFF00"), sp("#000000", "#FFFF00")},
			Notice:    LevelSpec{sp("#000000", "#00FFFF"), sp("#00FFFF", "")},
			Info:      LevelSpec{sp("#000000", "#00FF00"), sp("#00FF00", "")},
			Debug:     LevelSpec{sp("#FFFFFF", "#FF5500"), sp("#FFFFFF", "#FF5500")},
		},
	}
}

// merge overlays o (what the file set) onto c (defaults).
func (c *Config) merge(o Config) {
	set := func(dst *string, v string) {
		if v != "" {
			*dst = v
		}
	}
	set(&c.Datetime, o.Datetime)
	set(&c.Host, o.Host)
	set(&c.App, o.App)
	set(&c.PID, o.PID)
	set(&c.Channel, o.Channel)
	set(&c.Extra, o.Extra)
	set(&c.Trace, o.Trace)
	d, s := c.Levels.at(), o.Levels.at()
	for i := range d {
		if s[i].Level != nil {
			d[i].Level = s[i].Level
		}
		if s[i].Text != nil {
			d[i].Text = s[i].Text
		}
	}
}

// resolvePath: -config flag > $LOGRECV_CONFIG > <user config dir>/logrecv/config.json
//
//	Windows: %AppData%\logrecv\config.json
//	Linux:   ~/.config/logrecv/config.json ($XDG_CONFIG_HOME)
//	macOS:   ~/Library/Application Support/logrecv/config.json
func resolvePath(flagVal string) (string, error) {
	if flagVal != "" {
		return filepath.Abs(flagVal)
	}
	if v := os.Getenv("LOGRECV_CONFIG"); v != "" {
		return v, nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine config dir: %w (use -config or LOGRECV_CONFIG)", err)
	}
	return filepath.Join(dir, "logrecv", "config.json"), nil
}

// loadConfig returns defaults if the file is missing or empty.
func loadConfig(path string) (Config, error) {
	cfg := defaultConfig()
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf")) // Notepad BOM
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var fc Config
	if err := dec.Decode(&fc); err != nil {
		if errors.Is(err, io.EOF) {
			return cfg, nil
		}
		return cfg, fmt.Errorf("%s: %w", path, err)
	}
	cfg.merge(fc)
	return cfg, nil
}

func writeFileAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".logrecv-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Chmod(name, 0o644); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Rename(name, path); err != nil {
		os.Remove(name)
		return err
	}
	return nil
}

// Theme is the compiled, ready-to-paint form of Config.
type Theme struct {
	datetime, host, app, pid, channel, extra, trace Style
	level, text                                     [8]Style
}

func compile(c Config, m colorMode) (*Theme, error) {
	var errs []error
	st := func(name, fg, bg string) Style {
		s, err := newStyle(m, fg, bg)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", name, err))
		}
		return s
	}
	t := &Theme{
		datetime: st("datetime", c.Datetime, ""),
		host:     st("host", c.Host, ""),
		app:      st("app", c.App, ""),
		pid:      st("pid", c.PID, ""),
		channel:  st("channel", c.Channel, ""),
		extra:    st("extra", c.Extra, ""),
		trace:    st("trace", c.Trace, ""),
	}
	for i, l := range c.Levels.at() {
		lv, tx := l.Level, l.Text
		if lv == nil {
			lv = &StyleSpec{}
		}
		if tx == nil {
			tx = &StyleSpec{}
		}
		t.level[i] = st("levels."+levelKeys[i]+".level", lv.FG, lv.BG)
		t.text[i] = st("levels."+levelKeys[i]+".text", tx.FG, tx.BG)
	}
	return t, errors.Join(errs...)
}

// Store holds the live theme and hot-reloads it when the config file changes.
type Store struct {
	path string
	mode colorMode
	cur  atomic.Pointer[Theme]
	sig  fileSig
}

type fileSig struct {
	mod  time.Time
	size int64
	ok   bool
}

func statSig(path string) fileSig {
	fi, err := os.Stat(path)
	if err != nil {
		return fileSig{}
	}
	return fileSig{fi.ModTime(), fi.Size(), true}
}

func (s *Store) theme() *Theme { return s.cur.Load() }

func (s *Store) load() error {
	cfg, err := loadConfig(s.path)
	if err != nil {
		return err
	}
	th, err := compile(cfg, s.mode)
	if err != nil {
		return fmt.Errorf("%s: %w", s.path, err)
	}
	s.cur.Store(th)
	return nil
}

// watch polls the file; on a bad edit it keeps the previous theme.
func (s *Store) watch(done <-chan struct{}, logf func(string, ...any)) {
	s.sig = statSig(s.path)
	t := time.NewTicker(500 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-done:
			return
		case <-t.C:
		}
		sig := statSig(s.path)
		if sig == s.sig {
			continue
		}
		s.sig = sig
		if err := s.load(); err != nil {
			logf("config reload failed, keeping previous colors:\n%v", err)
			continue
		}
		if sig.ok {
			logf("config reloaded: %s", s.path)
		} else {
			logf("config removed, using default colors")
		}
	}
}
