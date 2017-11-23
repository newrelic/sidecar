package discovery

import (
	"regexp"

	log "github.com/sirupsen/logrus"
	"github.com/fsouza/go-dockerclient"
)

type ServiceNamer interface {
	ServiceName(*docker.APIContainers) string
}

// A ServiceNamer that uses a regex to match against the service name
// or else uses the image as the service name.
type RegexpNamer struct {
	ServiceNameMatch string
	expression       *regexp.Regexp
}

// Return a properly regex-matched name for the service, or failing that,
// the Image ID which we use to stand in for the name of the service.
func (r *RegexpNamer) ServiceName(container *docker.APIContainers) string {
	if container == nil {
		log.Warn("ServiceName() called with nil service passed!")
		return ""
	}

	if r.expression == nil {
		var err error

		r.expression, err = regexp.Compile(r.ServiceNameMatch)
		if err != nil {
			log.Errorf("Invalid regex, can't compile: %s", r.ServiceNameMatch)
			return container.Image
		}
	}

	var svcName string

	toMatch := []byte(container.Names[0])
	matches := r.expression.FindSubmatch(toMatch)
	if len(matches) < 1 {
		svcName = container.Image
	} else {
		svcName = string(matches[1])
	}

	return svcName
}

// A ServiceNamer that uses a name provided in a Docker label as the name
// for the service.
type DockerLabelNamer struct {
	Label string
}

// Return the value of the configured Docker label, or default to the image
// name.
func (d *DockerLabelNamer) ServiceName(container *docker.APIContainers) string {
	if container == nil {
		log.Warn("ServiceName() called with nil service passed!")
		return ""
	}

	for label, value := range container.Labels {
		if label == d.Label {
			return value
		}
	}

	log.Debugf(
		"Found container with no '%s' label: %s (%s), returning '%s'", d.Label,
		container.ID, container.Names[0], container.Image,
	)

	return container.Image
}
