package haproxy

import (
	"io"
	"os"
	"os/exec"
	"path"
	"regexp"
	"sort"
	"strconv"
	"text/template"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/newrelic/sidecar/catalog"
	"github.com/newrelic/sidecar/service"
)

type portset map[string]string
type portmap map[string]portset

// Configuration and state for the HAproxy management module
type HAproxy struct {
	ReloadCmd  string
	VerifyCmd  string
	BindIP     string
	Template   string
	ConfigFile string
	PidFile    string
}

// Constructs a properly configure HAProxy and returns a pointer to it
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

	for name, svcList := range services {
		if _, ok := ports[name]; !ok {
			ports[name] = make(portset, 5)
		}

		for _, service := range svcList {
			for _, port := range service.Ports {
				// Currently only handle TCP, and we skip ports that aren't exported.
				// That's the effect of not specifying a ServicePort.
				if port.Type == "tcp" && port.ServicePort != 0 {
					svcPort := strconv.FormatInt(port.ServicePort, 10)
					internalPort := strconv.FormatInt(port.Port, 10)
					ports[name][svcPort] = internalPort
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

// Create an HAproxy config from the supplied ServicesState. Write it out to the
// supplied io.Writer interface. This gets a list from servicesWithPorts() and
// builds a list of unique ports for all services, then passes these to the
// template. Ports are looked up by the func getPorts().
func (h *HAproxy) WriteConfig(state *catalog.ServicesState, output io.Writer) {
	services := servicesWithPorts(state)
	ports := h.makePortmap(services)

	data := struct {
		Services map[string][]*service.Service
	}{
		Services: services,
	}

	funcMap := template.FuncMap{
		"now": time.Now().UTC,
		"getPorts": func(k string) map[string]string {
			return ports[k]
		},
		"bindIP":       func() string { return h.BindIP },
		"sanitizeName": sanitizeName,
	}

	t, err := template.New("haproxy").Funcs(funcMap).ParseFiles(h.Template)
	if err != nil {
		log.Errorf("Error Parsing template '%s': %s", h.Template, err.Error())
		return
	}
	t.ExecuteTemplate(output, path.Base(h.Template), data)
}

// Execute a command and log the error, but bubble it up as well
func (h *HAproxy) run(command string) error {
	cmd := exec.Command("/bin/bash", "-c", command)
	err := cmd.Run()
	if err != nil {
		log.Errorf("Error running '%s': %s", command, err.Error())
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
	eventChannel := make(chan catalog.ChangeEvent, 2)
	state.AddListener(eventChannel)

	for {
		event := <-eventChannel
		log.Println("State change event from " + event.Hostname)
		h.WriteAndReload(state)
	}
}

// Write out the the HAproxy config and reload the service.
func (h *HAproxy) WriteAndReload(state *catalog.ServicesState) {
	outfile, err := os.Create(h.ConfigFile)
	if err != nil {
		log.Errorf("Unable to write to %s! (%s)", h.ConfigFile, err.Error())
		return
	}

	h.WriteConfig(state, outfile)
	if err := h.Verify(); err != nil {
		log.Errorf("Failed to verify HAproxy config! (%s)", err.Error())
		return
	}

	h.Reload()
}

// Like state.ByService() but only stores information for services which
// actually have public ports. Only matches services that have the same name
// and the same ports. Otherwise log an error.
func servicesWithPorts(state *catalog.ServicesState) map[string][]*service.Service {
	serviceMap := make(map[string][]*service.Service)

	state.EachServiceSorted(
		func(hostname *string, serviceId *string, svc *service.Service) {
			if len(svc.Ports) < 1 {
				return
			}

			// We only want things that are alive and healthy!
			if !svc.IsAlive() {
				return
			}

			svcName := state.ServiceName(svc)
			if _, ok := serviceMap[svcName]; !ok {
				serviceMap[svcName] = make([]*service.Service, 0, 3)
			}

			// If this is the first one, just add it to the list
			if len(serviceMap[svcName]) < 1 {
				serviceMap[svcName] = append(serviceMap[svcName], svc)
				return
			}

			// Otherwise we need to make sure the ServicePorts match
			match := serviceMap[svcName][0] // Get the first entry for comparison

			// Build up a sorted list of ServicePorts from the existing service
			portsToMatch := getSortedServicePorts(match)

			// Get the list of our ports
			portsWeHave := getSortedServicePorts(svc)

			// Compare the two sorted lists
			for i, port := range portsToMatch {
				if portsWeHave[i] != port {
					// TODO should we just add another service with this port added
					// to the name? We have to find out which port.
					log.Warnf("%s service from %s not added: non-matching ports!",
						state.ServiceName(svc), svc.Hostname)
					return
				}
			}

			// It was a match! Append to the list.
			serviceMap[svcName] = append(serviceMap[svcName], svc)
		},
	)

	return serviceMap
}

func getSortedServicePorts(svc *service.Service) []string {
	var portList []string
	for _, port := range svc.Ports {
		portList = append(portList, strconv.FormatInt(port.ServicePort, 10))
	}

	sort.Strings(portList)
	return portList
}
