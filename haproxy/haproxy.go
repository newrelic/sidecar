package haproxy

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"regexp"
	"sort"
	"strconv"
	"sync"
	"syscall"
	"text/template"
	"time"

	"github.com/Nitro/sidecar/catalog"
	"github.com/Nitro/sidecar/service"
	log "github.com/Sirupsen/logrus"
)

type portset map[string]string
type portmap map[string]portset

// Configuration and state for the HAproxy management module
type HAproxy struct {
	ReloadCmd      string `toml:"reload_cmd"`
	VerifyCmd      string `toml:"verify_cmd"`
	BindIP         string `toml:"bind_ip"`
	Template       string `toml:"template"`
	ConfigFile     string `toml:"config_file"`
	PidFile        string `toml:"pid_file"`
	User           string `toml:"user"`
	Group          string `toml:"group"`
	UseHostnames   bool   `toml:"use_hostnames"`
	eventChannel   chan catalog.ChangeEvent
	signalsHandled bool
	sigLock        sync.Mutex
	sigStopChan    chan struct{}
}

// Constructs a properly configured HAProxy and returns a pointer to it
func New(configFile string, pidFile string) *HAproxy {
	reloadCmd := "haproxy -f " + configFile + " -p " + pidFile + " `[[ -f " + pidFile + " ]] && echo \"-sf $(cat " + pidFile + ")\"]]`"
	verifyCmd := "haproxy -c -f " + configFile

	proxy := HAproxy{
		ReloadCmd:  reloadCmd,
		VerifyCmd:  verifyCmd,
		Template:   "views/haproxy.cfg",
		ConfigFile: configFile,
		PidFile:    pidFile,
	}

	return &proxy
}

// Returns a map of ServicePort:Port pairs
func (h *HAproxy) makePortmap(services map[string][]*service.Service) portmap {
	ports := make(portmap)

	for svcName, svcList := range services {
		if _, ok := ports[svcName]; !ok {
			ports[svcName] = make(portset, 5)
		}

		for _, service := range svcList {
			for _, port := range service.Ports {
				// Currently only handle TCP, and we skip ports that aren't exported.
				// That's the effect of not specifying a ServicePort.
				if port.Type == "tcp" && port.ServicePort != 0 {
					svcPort := strconv.FormatInt(port.ServicePort, 10)
					internalPort := strconv.FormatInt(port.Port, 10)
					ports[svcName][svcPort] = internalPort
				}
			}
		}
	}

	return ports
}

// Clean up image names for writing as HAproxy frontend and backend entries
func sanitizeName(image string) string {
	replace := regexp.MustCompile("[^a-z0-9-]")
	return replace.ReplaceAllString(image, "-")
}

// Find a matching Port when given a ServicePort
func findPortForService(svcPort string, svc *service.Service) string {
	matchPort, err := strconv.ParseInt(svcPort, 10, 64)
	if err != nil {
		log.Errorf("Invalid value from template ('%s') can't parse as int64: %s", svcPort, err.Error())
		return "-1"
	}

	for _, port := range svc.Ports {
		if port.ServicePort == matchPort {
			internalPort := strconv.FormatInt(port.Port, 10)
			return internalPort
		}
	}

	return "-1"
}

// Find the matching IP address when given a ServicePort
func (h *HAproxy) findIpForService(svcPort string, svc *service.Service) string {
	// We can turn off using IP addresses in the config, which is sometimes
	// necessary (e.g. w/Docker for Mac).
	if h.UseHostnames {
		return svc.Hostname
	}

	matchPort, err := strconv.ParseInt(svcPort, 10, 64)
	if err != nil {
		log.Errorf("Invalid value from template ('%s') can't parse as int64: %s", svcPort, err.Error())
		return "-1"
	}

	for _, port := range svc.Ports {
		if port.ServicePort == matchPort {
			return port.IP
		}
	}

	// This defaults to the previous behavior of templating the hostname
	// instead of the IP address. This relies on haproxy being able to
	// resolve the hostname (which means non-FQDN hostnames are a hazard).
	// Ideally this never happens for clusters that have IP addresses defined.
	return svc.Hostname
}

// Create an HAproxy config from the supplied ServicesState. Write it out to the
// supplied io.Writer interface. This gets a list from servicesWithPorts() and
// builds a list of unique ports for all services, then passes these to the
// template. Ports are looked up by the func getPorts().
func (h *HAproxy) WriteConfig(state *catalog.ServicesState, output io.Writer) error {

	state.RLock()
	services := servicesWithPorts(state)
	ports := h.makePortmap(services)
	modes := getModes(state)
	state.RUnlock()

	data := struct {
		Services map[string][]*service.Service
		User     string
		Group    string
	}{
		Services: services,
		User:     h.User,
		Group:    h.Group,
	}

	funcMap := template.FuncMap{
		"now": time.Now().UTC,
		"getMode": func(k string) string {
			return modes[k]
		},
		"getPorts": func(k string) map[string]string {
			return ports[k]
		},
		"portFor":      findPortForService,
		"ipFor":        h.findIpForService,
		"bindIP":       func() string { return h.BindIP },
		"sanitizeName": sanitizeName,
	}

	t, err := template.New("haproxy").Funcs(funcMap).ParseFiles(h.Template)
	if err != nil {
		return fmt.Errorf("Error Parsing template '%s': %s", h.Template, err.Error())
	}

	// We write into a buffer so disk IO doesn't hold up the whole state lock
	buf := bytes.NewBuffer(make([]byte, 0, 65535))
	state.RLock()
	err = t.ExecuteTemplate(buf, path.Base(h.Template), data)
	state.RUnlock()
	if err != nil {
		return fmt.Errorf("Error executing template '%s': %s", h.Template, err.Error())
	}

	// This is the potentially slowest bit, do it outside the critical section
	io.Copy(output, buf)

	return nil
}

// notifySignals swallows a bunch of signals that get sent to us when running into
// an error from HAproxy. If we didn't swallow these, the process would potentially
// stop when the signals are propagated by the sub-shell.
func (h *HAproxy) swallowSignals() {
	// from HAproxy which propagate.
	sigChan := make(chan os.Signal, 10)

	// Used to stop the goroutine
	h.sigStopChan = make(chan struct{})

	go func() {
		for {
			select {
			case <-sigChan:
				// swallow signal
			case <-h.sigStopChan:
				break
			}
		}
	}()

	signal.Notify(sigChan, syscall.SIGSTOP, syscall.SIGTSTP, syscall.SIGTTIN, syscall.SIGTTOU)
}

// ResetSignals unhooks our signal handler from the signals the sub-commands
// initiate. This is potentially destructive if other places in the program have
// hooked to the same signals! Affected signals are SIGSTOP, SIGTSTP, SIGTTIN, SIGTTOU.
func (h *HAproxy) ResetSignals() {
	h.sigLock.Lock()
	signal.Reset(syscall.SIGSTOP, syscall.SIGTSTP, syscall.SIGTTIN, syscall.SIGTTOU)
	select {
	case h.sigStopChan <- struct{}{}: // nothing
	default:
	}

	h.sigLock.Unlock()
}

// Execute a command and bubble up the error. Includes locking behavior which means
// that only one of these can be running at once.
func (h *HAproxy) run(command string) error {

	cmd := exec.Command("/bin/bash", "-c", command)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	// The end effect of this signal handling requirement is that we can only run _one_
	// command at a time. This is totally fine for HAproxy.
	h.sigLock.Lock()
	defer h.sigLock.Unlock()

	if !h.signalsHandled {
		log.Info("Setting up signal handlers")
		h.swallowSignals()
		h.signalsHandled = true
	}

	err := cmd.Run()

	if err != nil {
		err = fmt.Errorf("Error running '%s': %s\n%s\n%s", command, err, stdout, stderr)
	}

	return err
}

// Run the HAproxy reload command to load the new config and restart.
// Best to use a command with -sf specified to keep the connections up.
func (h *HAproxy) Reload() error {
	return h.run(h.ReloadCmd)
}

// Run HAproxy with the verify command that will check the validity of
// the current config. Used to gate a Reload() so we don't load a bad
// config and tear everything down.
func (h *HAproxy) Verify() error {
	return h.run(h.VerifyCmd)
}

// Watch the state of a ServicesState struct and generate a new proxy
// config file (haproxy.ConfigFile) when the state changes. Also notifies
// the service that it needs to reload once the new file has been written
// and verified.
func (h *HAproxy) Watch(state *catalog.ServicesState) {
	h.eventChannel = make(chan catalog.ChangeEvent, 2)
	state.AddListener(h)

	for event := range h.eventChannel {
		log.Println("State change event from " + event.Service.Hostname)
		err := h.WriteAndReload(state)
		if err != nil {
			log.Error(err.Error())
		}
	}

	state.RemoveListener(h.Name())
}

// Write out the the HAproxy config and reload the service.
func (h *HAproxy) WriteAndReload(state *catalog.ServicesState) error {
	if h.ConfigFile == "" {
		return fmt.Errorf("Trying to write HAproxy config, but no filename specified!")
	}

	outfile, err := os.Create(h.ConfigFile)
	if err != nil {
		return fmt.Errorf("Unable to write to %s! (%s)", h.ConfigFile, err.Error())
	}

	if err := h.WriteConfig(state, outfile); err != nil {
		return err
	}

	if err = h.Verify(); err != nil {
		return fmt.Errorf("Failed to verify HAproxy config! (%s)", err.Error())
	}

	return h.Reload()
}

// Name is part of the catalog.Listener interface. Returns the listener name.
func (h *HAproxy) Name() string {
	return "HAproxy"
}

// Chan is part of the catalog.Listener interface. Returns the channel we listen on.
func (h *HAproxy) Chan() chan catalog.ChangeEvent {
	return h.eventChannel
}

func getModes(state *catalog.ServicesState) map[string]string {
	modeMap := make(map[string]string)
	state.EachService(
		func(hostname *string, serviceId *string, svc *service.Service) {
			modeMap[svc.Name] = svc.ProxyMode
		},
	)
	return modeMap
}

// Like state.ByService() but only stores information for services which
// actually have public ports. Only matches services that have the same name
// and the same ports. Otherwise log an error.
func servicesWithPorts(state *catalog.ServicesState) map[string][]*service.Service {
	serviceMap := make(map[string][]*service.Service)

	state.EachService(
		func(hostname *string, serviceId *string, svc *service.Service) {
			if len(svc.Ports) < 1 {
				return
			}

			// We only want things that are alive and healthy!
			if !svc.IsAlive() {
				return
			}

			// If this is the first one, just set it
			if _, ok := serviceMap[svc.Name]; !ok {
				serviceMap[svc.Name] = []*service.Service{svc}
				return
			}

			// Otherwise we need to make sure the ServicePorts match
			match := serviceMap[svc.Name][0] // Get the first entry for comparison

			// Build up a sorted list of ServicePorts from the existing service
			portsToMatch := getSortedServicePorts(match)

			// Get the list of our ports
			portsWeHave := getSortedServicePorts(svc)

			// Compare the two sorted lists
			for i, port := range portsToMatch {
				if portsWeHave[i] != port {
					// TODO should we just add another service with this port added
					// to the name? We have to find out which port.
					log.Warnf("%s service from %s not added: non-matching ports! (%v vs %v)",
						svc.Name, svc.Hostname, port, portsWeHave[i])
					return
				}
			}

			// It was a match! Append to the list.
			serviceMap[svc.Name] = append(serviceMap[svc.Name], svc)
		},
	)

	return serviceMap
}

func getSortedServicePorts(svc *service.Service) []string {
	// Allocate once, with exact length
	portList := make([]string, len(svc.Ports))
	for i, port := range svc.Ports {
		portList[i] = strconv.FormatInt(port.ServicePort, 10)
	}

	sort.Strings(portList)
	return portList
}
