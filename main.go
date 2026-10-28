// Package main implements a service which communicates with a LightwaveRF Link (LWL) to monitor battery levels of peripherals
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"sync"
	"text/template"
	"time"

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

var tmplTRVStatus = template.Must(template.New("TRV Status").Parse(`
    batt: {{.Batt}}
   cTarg: {{.CTarg}}
   cTemp: {{.CTemp}}
  output: {{.Output}}

  JSON: {{.}}

`))

func (c *config) String() string {
	bits := []string{}
	buf := strings.Builder{}
	for serial := range c.names {
		name := c.names[serial]
		buf.Reset()
		if response, found := c.status[serial]; found {
			err := tmplTRVStatus.Execute(&buf, response)
			if err != nil {
				panic(err)
			}
		} else {
			buf.WriteString("[no report]")
		}
		entry := fmt.Sprintf("%s:\n%s", name, buf.String())
		bits = append(bits, entry)
	}
	report := strings.Join(bits, "\n")
	return report
}

func CallForHeat(cfh string, wantHeat bool) lwl.Command {
	// Check if id is a Room Device (i.e. relay, like JSJSLW821)
	var room int
	var dev int
	n, err := fmt.Sscanf(cfh, "R%dD%d", &room, &dev)
	if err != nil {
		slog.Error("Error from fmt.Sscanf", "cfh", cfh, "err", err)
	}

	if n == 0 {
		slog.Error("Not implemented: Call for heat devices other than relays (JSJSLW821)")
		os.Exit(1)
	}

	// Normalise (e.g. R05D01 -> R1D5)
	cfh = fmt.Sprintf("R%dD%d", room, dev)

	var cmd lwl.Command
	if wantHeat {
		slog.Info("Calling for heat", "cfh", cfh)
		cmd = lwl.CmdOpen.New(cfh)
	} else {
		cmd = lwl.CmdStop.New(cfh)
		slog.Info("Calling for stop heat", "cfh", cfh)
	}

	return cmd
}

func stdinLines(ctx context.Context) <-chan string {
	ch := make(chan string)
	b := make([]byte, 1)
	go func() {
		defer close(ch)
		for {
			n, err := os.Stdin.Read(b)
			if err != nil {
				slog.Error("stdin read", "err", err)
			}
			select {
			case ch <- string(b[:n]):
			case <-ctx.Done():
				return
			}
		}
	}()
	return ch
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

	// Signal handling
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
	defer stop()

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

	cfh := ""
	for id := range conf.names {
		if conf.names[id] == "Call for heat" {
			cfh = id
			break
		}
	}
	if cfh == "" {
		panic("Configuration file lacks 'Call for heat' entry")
	}
	slog.Info("Call for heat device is", "cfh", cfh)

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

	// // HACK: Start
	// cmd := lwl.CmdQueryRadiatorStatus.New("R1")
	// r, err := c.Do(ctx, cmd)
	// slog.Info("HACK", "r", r, "err", err)
	// // HACK: End
	// os.Exit(0)

	c.EnsureRegistered()

	// TODO: Respond to valve changes by calling for heat (.. or stopping)
	//
	// slog.Info("Testing relay controls")
	// cmd := CallForHeat(cfh, false)
	// r, err := c.Do(ctx, cmd)
	// if err != nil {
	// 	panic(err)
	// }
	// slog.Info("Response:", "r", r)
	// cmd = CallForHeat(cfh, true)
	// r, err = c.Do(ctx, cmd)
	// if err != nil {
	// 	panic(err)
	// }
	// slog.Info("Response:", "r", r)

	r, err := c.Do(ctx, lwl.CmdHubCall)
	if err != nil {
		slog.Error("HubCall failed", "err", err)
	}
	slog.Info("HubCall", "response", &r)

	// Run in background so we can process the responses too as we collect the
	// temperatures and valve output of every TRV
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		slog.Info("QueryAllRadiators() start")
		err = c.QueryAllRadiators(ctx)
		if err != nil {
			slog.Error("QueryAllRadiators failed", "err", err)
		}
		slog.Info("QueryAllRadiators() done")
	}()

	stdin := stdinLines(ctx)

	slog.Info("Starting main loop")
loop:
	for {
		select {
		case msg := <-msgs:
			name := conf.seen(msg)
			slog.Info("conf.seen()", "name", name, "msg", &msg)
			if lwl.CmdQueryRadiatorStatus.IsResponse(msg) {
				slog.Info("Deciding if we need heat...")

				// Decide if we want heating on or not
				cout := 0 // Cumulative output
				for serial := range conf.status {
					state := conf.status[serial]
					switch state.Fn {
					case "statusOn", "statusPush":
						cout += int(state.Output)
					}
				}
				cmd := CallForHeat(cfh, cout > 0)
				r, err = c.Do(ctx, cmd)
				if err != nil {
					slog.Error("Failed to set heating state", "err", err)
				}
				slog.Info("Response:", "r", r)
			}

		// TODO: Increase timeout (6h?)
		// TODO: Check for stale TRV status, and request update
		// TODO: Send alerts for unresponsive TRVs
		// TODO: Low battery alerts
		case <-time.After(1 * time.Hour):
			slog.Info("Timeout", "c", c, "c.Stats()", c.Stats())

		case <-ctx.Done():
			slog.Info("Exiting due to signal")
			break loop

		case <-stdin:
			// Display status
			slog.Info("Current status", "c", c, "c.Stats()", c.Stats(), "conf", conf)

		}

		err = conf.write()
		if err != nil {
			slog.Error("Failed to write out configuration file", "fn", configFile, "err", err)
			return
		}
	}
}
