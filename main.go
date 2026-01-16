// Package main implements a service which communicates with a LightwaveRF Link (LWL) to monitor battery levels of peripherals
package main

import (
	"context"
	"flag"
	"log/slog"
	"maps"
	"os"
	"os/signal"
	"strings"
	"sync"
	"time"

	"github.com/meermanr/LightwaveRF-go/lwl"

	"github.com/MatusOllah/slogcolor"
	"gopkg.in/yaml.v3"
)

const configFile = "config.yaml"

var isVerbose = flag.Bool("verbose", false, "Enable display of DEBUG log messages")
var wantDeregister = flag.Bool("unpair", false, "Unpair from LightwaveLink")

type config struct {
	mu     sync.RWMutex            // Mutex
	names  map[string]string       // Serial -> Name, e.g. "24C702" -> "Master Bedroom"
	status map[string]lwl.Response // Serial -> most recent statusPush
	yaml   yaml.Node               // Decoded YAML, inc. comments
}

func (c *config) load(fn string) error {
	data, err := os.ReadFile(fn)
	if err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Decode into yaml.Node, preserving comments et  al
	if err := yaml.Unmarshal(data, &c.yaml); err != nil {
		return err
	}
	// Extract just the data
	if err := yaml.Unmarshal(data, &c.names); err != nil {
		return err
	}
	return nil
}

func (c *config) write(fn string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Find names not in original config
	newNames := maps.Clone(c.names)

	// Find (or create) root mapping of serial -> name
	var mapping *yaml.Node
	if len(c.yaml.Content) == 0 {
		// Add a mapping node
		mapping = &yaml.Node{
			Kind: yaml.MappingNode,
		}
		c.yaml.Content = append(c.yaml.Content, mapping)
	} else {
		mapping = c.yaml.Content[0]
	}

	// mapping.Content is a list of [key, value, key, value, ...]
	for i := 0; i < len(mapping.Content); i += 2 {
		k := mapping.Content[i]
		delete(newNames, k.Value)
	}

	if len(newNames) == 0 {
		slog.Debug("Not writing out config, as no new data to add", "fn", configFile)
		return nil
	}

	// Append missing names to YAML document
	for k, v := range newNames {
		yk := &yaml.Node{
			Kind:  yaml.ScalarNode,
			Value: k,
			Tag:   "!!str",
			Style: yaml.DoubleQuotedStyle,
		}
		yv := &yaml.Node{
			Kind:  yaml.ScalarNode,
			Value: v,
			Tag:   "!!str",
			Style: yaml.DoubleQuotedStyle,
		}
		mapping.Content = append(mapping.Content, yk, yv)
	}

	f, err := os.CreateTemp(".", strings.Join([]string{".", fn, "*"}, ""))
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())

	enc := yaml.NewEncoder(f)
	enc.SetIndent(2)
	defer enc.Close()

	if err := enc.Encode(&c.yaml); err != nil {
		return err
	}

	os.Rename(f.Name(), fn)
	return nil
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
	conf := config{}
	if err := conf.load(configFile); err != nil {
		switch {
		case os.IsNotExist(err):
			slog.Warn("Configuration file does not exist.", "fn", configFile)
		default:
			slog.Error("Unable to load configuration file", "fn", configFile, "err", err)
		}
	} else {
		slog.Debug("Loaded configuration.", "fn", configFile)
	}

	defer func() {
		if err := conf.write(configFile); err != nil {
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
		slog.Info("Deregister", "response", c.DoLegacy(lwl.CmdDeregister.String()))
	}

	c.EnsureRegistered()

	slog.Info("@H", "response", c.DoLegacy("@H"))

	// Signal handling
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
	defer stop()

	err := c.QueryAllRadiators(ctx)
	if err != nil {
		slog.Error("QueryAllRadiators", "err", err)
	}

	slog.Info("Starting main loop")
loop:
	for {
		select {
		case msg := <-msgs:
			name := conf.seen(msg)
			slog.Info("JSON Response", "name", name, "msg", &msg)
		case <-time.After(10 * time.Second):
			slog.Info("Timeout", "c", c, "c.Stats()", c.Stats())
			conf.write(configFile)
		case <-ctx.Done():
			slog.Info("Exiting due to signal")
			break loop
		}
	}
}
