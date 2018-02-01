package nbd

import (
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	// registers profiling HTTP Handlers
	"context"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
	"syscall"

	"github.com/sevlyar/go-daemon"
	"gopkg.in/yaml.v2"
)

/* Example configuration:

servers:
- protocol: tcp
  address: 127.0.0.1:6666
  exports:
  - name: foo
    driver: file
    path: /tmp/test
  - name: bar
    readonly: true
    driver: rbd
    rdbname: rbdbar
    timeout: 5s
- protocol: unix
  address: /var/run/nbd.sock
  exports:
  - name: baz
    driver: file
    readonly: false
    path: /tmp/baz
    sync: true
logging:
  syslogfacility: local1
*/

var (
	// Location of the config file on disk; overriden by flags
	configFile        string
	pidFile           string
	sendSignal        string
	foreground        bool
	pprof             bool
	registerFlagsOnce sync.Once
)

// RegisterFlags registers all NBD-Specific flags
func RegisterFlags() {
	registerFlagsOnce.Do(func() {
		flag.StringVar(&configFile, "c", "/etc/gonbdserver.conf", "Path to YAML config file")
		flag.StringVar(&pidFile, "p", "/var/run/gonbdserver.pid", "Path to PID file")
		flag.StringVar(&sendSignal, "s", "", "Send signal to daemon (either \"stop\" or \"reload\")")
		flag.BoolVar(&foreground, "f", false, "Run in foreground (not as daemon)")
		flag.BoolVar(&pprof, "pprof", false, "Run pprof")
	})
}

// Environment variables that can be used to
// overwrite some of the flags.
const (
	EnvConfigFile = "_GONBDSERVER_CONFFILE"
	EnvPIDFile    = "_GONBDSERVER_PIDFILE"
)

// Control structure is used to sync an async event
type Control struct {
	quit chan struct{}
}

// Config holds the config that applies to all servers (currently just logging), and an array of server configs
type Config struct {
	Servers []ServerConfig // array of server configs
}

// ServerConfig holds the config that applies to each server (i.e. listener)
type ServerConfig struct {
	Protocol        string         // protocol it should listen on (in net.Conn form)
	Address         string         // address to listen on
	DefaultExport   string         // name of default export
	Exports         []ExportConfig // array of configurations of exported items
	TLS             TLSConfig      // TLS configuration
	DisableNoZeroes bool           // Disable NoZereos extension
}

// ExportConfig holds the config for one exported item
type ExportConfig struct {
	Name               string                 // name of the export
	Description        string                 // description of export
	Driver             string                 // name of the driver
	ReadOnly           bool                   // true of the export should be opened readonly
	TLSOnly            bool                   // true if the export should only be served over TLS
	MinimumBlockSize   uint64                 // minimum block size
	PreferredBlockSize uint64                 // preferred block size
	MaximumBlockSize   uint64                 // maximum block size
	DriverParameters   DriverParametersConfig `yaml:",inline"` // driver parameters. These are an arbitrary map. Inline means they go aside teh foregoing
}

// TLSConfig has the configuration for TLS
type TLSConfig struct {
	KeyFile    string // path to TLS key file
	CertFile   string // path to TLS cert file
	ServerName string // server name
	CaCertFile string // path to certificate file
	ClientAuth string // client authentication strategy
	MinVersion string // minimum TLS version
	MaxVersion string // maximum TLS version
}

// DriverParametersConfig is an arbitrary map of other parameters in string format
type DriverParametersConfig map[string]string

var deletePrefix = regexp.MustCompile("gonbdserver:")
var replaceLevel = regexp.MustCompile("\\[[A-Z]+\\] ")

// ParseConfig parses the YAML configuration provided
func ParseConfig() (*Config, error) {
	buf, err := ioutil.ReadFile(configFile)
	if err != nil {
		return nil, err
	}

	c := &Config{}
	if err := yaml.Unmarshal(buf, c); err != nil {
		return nil, err
	}

	for i := range c.Servers {
		if c.Servers[i].Protocol == "" {
			c.Servers[i].Protocol = "tcp"
		}
		if c.Servers[i].Protocol == "tcp" && c.Servers[i].Address == "" {
			c.Servers[i].Protocol = fmt.Sprintf("0.0.0.0:%d", NBD_DEFAULT_PORT)
		}
	}

	return c, nil
}

// StartServer starts a single server.
//
// A parent context is given in which the listener runs, as well as a session context in which the sessions (connections) themselves run.
// This enables the sessions to be retained when the listener is cancelled on a SIGHUP
func StartServer(parentCtx context.Context, sessionParentCtx context.Context, sessionWaitGroup *sync.WaitGroup, logger Logger, s ServerConfig) {
	ctx, cancelFunc := context.WithCancel(parentCtx)

	defer func() {
		cancelFunc()
		logger.Infof("Stopping server %s:%s", s.Protocol, s.Address)
	}()

	logger.Infof("Starting server %s:%s", s.Protocol, s.Address)

	if l, err := NewListener(logger, s); err != nil {
		logger.Infof("Could not create listener for %s:%s: %v", s.Protocol, s.Address, err)
	} else {
		l.Listen(ctx, sessionParentCtx, sessionWaitGroup)
	}
}

// RunConfig - this is effectively the main entry point of the program
//
// We parse the config, then start each of the listeners, restarting them when we get SIGHUP, but being sure not to kill the sessions
func RunConfig(control *Control, logger Logger) {
	if logger == nil {
		logger = &StandardLogger{}
	}

	var sessionWaitGroup sync.WaitGroup
	ctx, cancelFunc := context.WithCancel(context.Background())
	defer func() {
		logger.Info("Shutting down")
		cancelFunc()
		sessionWaitGroup.Wait()
		logger.Info("Shutdown complete")
	}()

	intr := make(chan os.Signal, 1)
	term := make(chan os.Signal, 1)
	hup := make(chan os.Signal, 1)
	usr1 := make(chan os.Signal, 1)
	defer close(intr)
	defer close(term)
	defer close(hup)
	defer close(usr1)
	if control == nil {
		signal.Notify(intr, os.Interrupt)
		signal.Notify(term, syscall.SIGTERM)
		signal.Notify(hup, syscall.SIGHUP)
	}

	signal.Notify(usr1, syscall.SIGUSR1)
	go func() {
		for {
			select {
			case _, ok := <-usr1:
				if !ok {
					return
				}
				logger.Info("Run GC()")
				runtime.GC()
				logger.Info("GC() done")
				debug.FreeOSMemory()
				logger.Info("FreeOsMemory() done")
			}
		}
	}()

	for {
		var wg sync.WaitGroup
		configCtx, configCancelFunc := context.WithCancel(ctx)

		c, err := ParseConfig()
		if err != nil {
			logger.Infof("Cannot parse configuration file: %v\n", err)
			configCancelFunc()
			return
		}

		logger.Infof("Loaded configuration. Available backends: %s\n", strings.Join(GetBackendNames(), ", "))

		for _, s := range c.Servers {
			s := s // localise loop variable
			go func() {
				wg.Add(1)
				StartServer(configCtx, ctx, &sessionWaitGroup, logger, s)
				wg.Done()
			}()
		}

		select {
		case <-ctx.Done():
			logger.Info("Interrupted")
			configCancelFunc()
			return
		case <-intr:
			logger.Info("Interrupt signal received")
			configCancelFunc()
			return
		case <-term:
			logger.Info("Terminate signal received")
			configCancelFunc()
			return
		case <-control.quit:
			logger.Info("Programmatic quit received")
			configCancelFunc()
			return
		case <-hup:
			logger.Info("Reload signal received; reloading configuration which will be effective for new connections")
			configCancelFunc() // kill the listeners but not the sessions
			wg.Wait()
		}
	}
}

// Run defines the entry point of this nbd module.
// It creates the server in the foreground or as a deamon.
// It will create the listeners and server based on the config and defaults.
// Once that's all up and running, the service is ready to receive and reply to NBD Requests.
func Run(control *Control, logger Logger) {
	if control == nil {
		control = &Control{}
	}

	if pprof {
		runtime.MemProfileRate = 1
		go http.ListenAndServe(":8080", nil)
	}

	if logger == nil {
		logger = &StandardLogger{}
	}

	daemon.AddFlag(daemon.StringFlag(&sendSignal, "stop"), syscall.SIGTERM)
	daemon.AddFlag(daemon.StringFlag(&sendSignal, "reload"), syscall.SIGHUP)

	if daemon.WasReborn() {
		if val := os.Getenv(EnvConfigFile); val != "" {
			configFile = val
		}
		if val := os.Getenv(EnvPIDFile); val != "" {
			pidFile = val
		}
	}

	var err error
	if configFile, err = filepath.Abs(configFile); err != nil {
		logger.Fatalf("[CRIT] Error canonicalising config file path: %s", err)
	}
	if pidFile, err = filepath.Abs(pidFile); err != nil {
		logger.Fatalf("[CRIT] Error canonicalising pid file path: %v", err)
	}

	// check the configuration parses. We do nothing with this at this stage
	// but it eliminates a problem where the log of the configuration failing
	// is invisible when daemonizing naively (e.g. when no alternate log
	// destination is supplied) and the config file cannot be read
	if _, err := ParseConfig(); err != nil {
		logger.Fatalf("[CRIT] Cannot parse configuration file: %v", err)
		return
	}

	if foreground {
		RunConfig(control, logger)
		return
	}

	os.Setenv(EnvConfigFile, configFile)
	os.Setenv(EnvPIDFile, pidFile)

	// Define daemon context
	d := &daemon.Context{
		PidFileName: pidFile,
		PidFilePerm: 0644,
		Umask:       027,
	}

	// Send commands if needed
	if len(daemon.ActiveFlags()) > 0 {
		p, err := d.Search()
		if err != nil {
			logger.Fatalf("[CRIT] Unable send signal to the daemon - not running")
		}
		if err := p.Signal(syscall.Signal(0)); err != nil {
			logger.Fatalf("[CRIT] Unable send signal to the daemon - not running, perhaps PID file is stale")
		}
		daemon.SendCommands(p)
		return
	}

	if !daemon.WasReborn() {
		if p, err := d.Search(); err == nil {
			if err := p.Signal(syscall.Signal(0)); err == nil {
				logger.Fatalf("[CRIT] Daemon is already running (pid %d)", p.Pid)
			} else {
				logger.Infof("Removing stale PID file %s", pidFile)
				os.Remove(pidFile)
			}
		}
	}

	// Process daemon operations - send signal if present flag or daemonize
	child, err := d.Reborn()
	if err != nil {
		logger.Fatalf("[CRIT] Daemonize: %s", err)
	}
	if child != nil {
		return
	}

	defer func() {
		d.Release()
		// for some reason this is not removing the pid file
		os.Remove(pidFile)
	}()

	RunConfig(control, logger)
}
