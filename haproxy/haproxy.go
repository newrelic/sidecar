package haproxy

import (
	"io"
	"os"
	"os/exec"
	"path"
	"reflect"
	"regexp"
	"strconv"
	"text/template"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/newrelic/bosun/catalog"
	"github.com/newrelic/bosun/service"
)

type portset map[string]struct{}
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
	reloadCmd := "haproxy -f " + configFile + " -p " + pidFile + " -sf $(cat " + pidFile + ")"
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

// Returns a map of sets where each set is a list of ports bound
// by that service.
func (h *HAproxy) makePortmap(services map[string][]*service.Service) portmap {
	ports := make(portmap)

	for name, svcList := range services {
		if _, ok := ports[name]; !ok {
			ports[name] = make(portset, 5)
		}

		for _, service := range svcList {
			for _, svcPort := range service.Ports {
				if svcPort.Type == "tcp" {
					ports[name][strconv.FormatInt(svcPort.Port, 10)] = struct{}{}
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
		"getPorts": func(k string) []string {
			var keys []string
			for key, _ := range ports[k] {
				keys = append(keys, key)
			}
			return keys
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
		outfile, err := os.Create(h.ConfigFile)
		if err != nil {
			log.Errorf("Unable to write to %s! (%s)", h.ConfigFile, err.Error())
			continue
		}

		h.WriteConfig(state, outfile)
		if err := h.Verify(); err != nil {
			log.Errorf("Failed to verify HAproxy config! (%s)", err.Error())
			continue
		}

		h.Reload()
	}
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

			if len(serviceMap[svcName]) < 1 {
				serviceMap[svcName] = append(serviceMap[svcName], svc)
				return
			}

			match := serviceMap[svcName][0]
			// DeepEqual is slow, so we only do it when we have to
			if match != nil && reflect.DeepEqual(match.Ports, svc.Ports) {
				serviceMap[svcName] = append(serviceMap[svcName], svc)
			} else {
				// TODO should we just add another service with this port added
				// to the name? We have to find out which port.
				log.Warnf("%s service from %s not added: non-matching ports!",
					state.ServiceName(svc), svc.Hostname)
			}
		},
	)

	return serviceMap
}
