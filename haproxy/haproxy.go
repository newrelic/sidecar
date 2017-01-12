package haproxy

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
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
	ReloadCmd    string `toml:"reload_cmd"`
	VerifyCmd    string `toml:"verify_cmd"`
	BindIP       string `toml:"bind_ip"`
	TemplateDir  string `toml:"template_dir"`
	ConfigFile   string `toml:"config_file"`
	PidFile      string `toml:"pid_file"`
	User         string `toml:"user"`
	Group        string `toml:"group"`
	eventChannel chan catalog.ChangeEvent
}

// Constructs a properly configured HAProxy and returns a pointer to it
func New(configFile string, pidFile string) *HAproxy {
	reloadCmd := "haproxy -f " + configFile + " -p " + pidFile + " `[[ -f " + pidFile + " ]] && echo \"-sf $(cat " + pidFile + ")\"]]`"
	verifyCmd := "haproxy -c -f " + configFile

	proxy := HAproxy{
		ReloadCmd:   reloadCmd,
		VerifyCmd:   verifyCmd,
		TemplateDir: "views/haproxy",
		ConfigFile:  configFile,
		PidFile:     pidFile,
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

// Templating function used to pass more than one var to a sub-template
func dict(values ...interface{}) (map[string]interface{}, error) {
	if len(values)%2 != 0 {
		return nil, errors.New("invalid dict call")
	}

	dict := make(map[string]interface{}, len(values)/2)
	for i := 0; i < len(values); i += 2 {
		key, ok := values[i].(string)
		if !ok {
			return nil, errors.New("dict keys must be strings")
		}
		dict[key] = values[i+1]
	}
	return dict, nil
}

// Create an HAproxy config from the supplied ServicesState. Write it out to the
// supplied io.Writer interface. This gets a list from servicesWithPorts() and
// builds a list of unique ports for all services, then passes these to the
// template. Ports are looked up by the func getPorts().
func (h *HAproxy) WriteConfig(state *catalog.ServicesState, output io.Writer) error {

	state.RLock()
	services := servicesWithPorts(state)
	ports := h.makePortmap(services)
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

	var t *template.Template

	funcMap := template.FuncMap{
		"bindIP":   func() string { return h.BindIP },
		"dict":     dict,
		"getPorts": func(k string) map[string]string { return ports[k] },
		"render": func(svcs []*service.Service, partial string, dict map[string]interface{}) string {
			if len(svcs) < 1 {
				return ""
			}
			return render(svcs[0], partial, dict, t)
		},
		"now":          time.Now().UTC,
		"portFor":      findPortForService,
		"sanitizeName": sanitizeName,
	}

	templates, err := filepath.Glob(path.Join(h.TemplateDir, "*.cfg"))
	if err != nil {
		return fmt.Errorf("Error reading template dir '%s': '%s'", h.TemplateDir, err.Error())
	}

	t, err = template.New("haproxy").Funcs(funcMap).ParseFiles(templates...)
	if err != nil {
		return fmt.Errorf("Error Parsing templates from '%s': %s", h.TemplateDir, err.Error())
	}

	// We write into a buffer so disk IO doesn't hold up the whole state lock
	buf := bytes.NewBuffer(make([]byte, 65536))
	state.RLock()
	err = t.ExecuteTemplate(buf, path.Base("haproxy.cfg"), data)
	state.RUnlock()
	if err != nil {
		return fmt.Errorf("Error executing templates from '%s': %s", h.TemplateDir, err.Error())
	}

	// This is the potentially slowest bit, do it outside the critical section
	io.Copy(output, buf)

	return nil
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
	h.eventChannel = make(chan catalog.ChangeEvent, 2)
	state.AddListener(h)

	for event := range h.eventChannel {
		log.Println("State change event from " + event.Service.Hostname)
		err := h.WriteAndReload(state)
		if err != nil {
			log.Error(err.Error())
		}
	}

	// TODO this should really clean up the listener
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

	h.Reload()

	return nil
}

// Name is part of the catalog.Listener interface. Returns the listener name.
func (h *HAproxy) Name() string {
	return "HAproxy"
}

// Chan is part of the catalog.Listener interface. Returns the channel we listen on.
func (h *HAproxy) Chan() chan catalog.ChangeEvent {
	return h.eventChannel
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

			if _, ok := serviceMap[svc.Name]; !ok {
				serviceMap[svc.Name] = make([]*service.Service, 0, 3)
			}

			// If this is the first one, just add it to the list
			if len(serviceMap[svc.Name]) < 1 {
				serviceMap[svc.Name] = append(serviceMap[svc.Name], svc)
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

// Return the template for a service and partial (section)
func render(svc *service.Service, partial string, dict map[string]interface{}, t *template.Template) string {
	profile := svc.Profile
	buf := bytes.NewBuffer(make([]byte, 16384))
	err := t.ExecuteTemplate(buf, profile+"-"+partial+".cfg", dict)
	if err != nil {
		log.Errorf("Error rendering partial %s for service %s: %s", partial, svc.Name, err.Error())
		return ""
	}

	return string(buf.Bytes())
}

func getSortedServicePorts(svc *service.Service) []string {
	var portList []string
	for _, port := range svc.Ports {
		portList = append(portList, strconv.FormatInt(port.ServicePort, 10))
	}

	sort.Strings(portList)
	return portList
}
