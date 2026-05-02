// Package main implements a service which communicates with a LightwaveRF Link (LWL) to monitor battery levels of peripherals
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/meermanr/LightwaveRF-go/lwl"

	"github.com/MatusOllah/slogcolor"
	flag "github.com/spf13/pflag"
	"gopkg.in/yaml.v3"
)

const configFile = "config.yaml"

var isVerbose = flag.BoolP("verbose", "v", false, "Enable display of DEBUG log messages")
var wantDeregister = flag.Bool("unpair", false, "Unpair from LightwaveLink")

type config struct {
	mu        sync.RWMutex            // Mutex
	names     map[string]string       // Serial -> Name, e.g. "24C702" -> "Master Bedroom"
	status    map[string]lwl.Response // Serial -> most recent statusPush
	yaml      yaml.Node               // Decoded YAML, inc. comments
	fn        string                  // File name, e.g. config.yaml
	persisted map[string]bool         // Serial -> bool, tracks which names are present in fn
}

func (c *config) load(fn string) error {
	c.fn = fn
	data, err := os.ReadFile(c.fn)
	if err != nil {
		return fmt.Errorf("failed to load configuration file %v: %w", c.fn, err)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Decode into yaml.Node, preserving comments et  al
	if err := yaml.Unmarshal(data, &c.yaml); err != nil {
		return fmt.Errorf("failed to parse configuration file %v: %w", c.fn, err)
	}

	// Extract just the data
	if err := yaml.Unmarshal(data, &c.names); err != nil {
		return fmt.Errorf("failed to parse configuration file %v: %w", c.fn, err)
	}

	// Mark names which are already persisted in the configuration file
	for n := range c.names {
		c.persisted[n] = true
	}

	return nil
}

// write appends new devices to a configuration file previously read by
// `.load()`
func (c *config) write() (err error) {
	// Desired YAML format is a flat mapping from serial string to name string,
	// e.g.:
	//
	// 24C702: Master bedroom
	// 47C702: Front door

	// TODO: Atomically replace config, so if things go wrong the original is
	// preserved
	c.mu.Lock()
	defer c.mu.Unlock()

	newNames := make(map[string]string, 10)
	for n := range c.names {
		_, found := c.persisted[n]
		if !found {
			value := c.names[n]
			newNames[n] = value
		}
	}

	if len(newNames) == 0 {
		slog.Debug("Not writing out config, as no new data to add", "fn", c.fn)
		return nil
	}

	spew.Dump("newNames", newNames)

	// Append missing names to YAML document
	new_yaml_data, err := yaml.Marshal(newNames)
	if err != nil {
		return fmt.Errorf("failed to encode new names, %v: %w", newNames, err)
	}

	f, err := os.CreateTemp(".", "."+c.fn+"*")
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			e := os.Remove(f.Name())
			if e != nil {
				slog.Error("failed to clean-up temporary file during aborted configuration file write", "err", e)
			}
		}
	}()

	// Copy original config to temporary file
	orig_yaml_data, err := os.ReadFile(c.fn)
	switch {
	case errors.Is(err, os.ErrNotExist):
		// Ignore. We'll simply make a new config from scratch
	case err != nil:
		return fmt.Errorf("failed to read original configuration file: %w", err)
	}

	_, err = f.Write(orig_yaml_data)
	if err != nil {
		return fmt.Errorf("failed to copy original config data before appending: %w", err)
	}

	// Append new data to temporary file
	_, err = f.Write(new_yaml_data)
	if err != nil {
		return fmt.Errorf("failed to write out encoded YAML data: %w", err)
	}

	// Atomically replace configuration file
	err = os.Rename(f.Name(), c.fn)
	if err != nil {
		return fmt.Errorf("failed to write out new config: %w", err)
	}

	for n := range newNames {
		c.persisted[n] = true
	}

	return nil
}

func NewConfig() *config {
	return &config{
		names:     make(map[string]string),
		status:    make(map[string]lwl.Response),
		persisted: make(map[string]bool),
	}
}

// seen records the given status, and returns the name entry from the
// configuration file (which may be empty)
func (c *config) seen(status lwl.Response) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if status.Serial == "" {
		return ""
	}
	name, found := c.names[status.Serial]
	if !found {
		name = "[New]"
		c.names[status.Serial] = "name"
	}
	if c.status == nil {
		c.status = make(map[string]lwl.Response)
	}
	c.status[status.Serial] = status
	return name
}

func main() {
	// Command line arguments
	flag.Parse()

	// Logging
	opts := slogcolor.DefaultOptions
	switch *isVerbose {
	case true:
		opts.Level = slog.LevelDebug
	case false:
		opts.Level = slog.LevelInfo
	}
	slog.SetDefault(slog.New(slogcolor.NewHandler(os.Stderr, opts)))
	slog.Debug("Debug messages look like this")

	// Config
	conf := NewConfig()
	if err := conf.load(configFile); err != nil {
		switch {
		case errors.Is(err, os.ErrNotExist):
			slog.Warn("Configuration file does not exist, will create later.", "fn", configFile)
		default:
			slog.Error("unable to load configuration file", "fn", configFile, "err", err)
		}
	} else {
		slog.Debug("Loaded configuration.", "fn", configFile)
	}

	defer func() {
		if err := conf.write(); err != nil {
			slog.Error("Error writing out configuration file", "fn", configFile, "err", err)
		} else {
			slog.Info("Wrote out config", "fn", configFile)
		}
	}()

	// LightwaveLink
	c := lwl.New()
	msgs := make(chan lwl.Response, 10)
	sid := c.Subscribe("", msgs, nil)
	defer c.Unsubscribe(sid)
	go c.Listen()

	if *wantDeregister {
		slog.Info("Deregister", "response", c.DoLegacy(lwl.CmdDeregister))
	}

	c.EnsureRegistered()

	// Signal handling
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
	defer stop()

	r, err := c.Do(ctx, lwl.CmdHubCall)
	if err != nil {
		slog.Error("HubCall failed", "err", err)
	}
	slog.Info("HubCall", "response", &r)

	go func() {
		slog.Info("QueryAllRadiators() start")
		err = c.QueryAllRadiators(ctx)
		if err != nil {
			slog.Error("QueryAllRadiators failed", "err", err)
		}
	}()

	slog.Info("Starting main loop")
loop:
	for {
		select {
		case msg := <-msgs:
			name := conf.seen(msg)
			slog.Info("JSON Response", "name", name, "msg", &msg)
		case <-time.After(10 * time.Second):
			slog.Info("Timeout", "c", c, "c.Stats()", c.Stats())
			err = conf.write()
			if err != nil {
				slog.Error("Failed to write out configuration file", "fn", configFile, "err", err)
				return
			}

		case <-ctx.Done():
			slog.Info("Exiting due to signal")
			break loop
		}
	}
}
