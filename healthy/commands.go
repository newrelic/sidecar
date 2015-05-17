// These are types that conform to the Checker interface
// and can be assigned to a Check for watching service
// health.
package healthy

import (
	"errors"
	"log"
	"net/http"
	"os/exec"
	"strings"
)

// A Checker that makes an HTTP get call and expects to get
// a 200-299 back as success. Anything else is considered
// a failure. The URL to hit is passed ass the args to the
// Run method.
type HttpGetCmd struct{}

func (h *HttpGetCmd) Run(args string) (int, error) {
	resp, err := http.Get(args)
	if resp == nil {
		return UNKNOWN, errors.New("No body from HTTP response!")
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return HEALTHY, nil
	}

	return SICKLY, err
}

// A Checker that works with Nagios checks or other simple
// external tools. It expects a 0 exit code from the command
// that was run. Anything else is considered to be SICKLY.
// The command is passed as the args to the Run method. The
// command will be executed without a shell wrapper to keep
// the call as lean as possible in the majority case. If you
// need a shell you must invoke it yourself.
type ExternalCmd struct{}

func (e *ExternalCmd) Run(args string) (int, error) {
	cliArgs := strings.Split(args, " ")
	cmd := exec.Command(cliArgs[0], cliArgs[1:]...)

	output, err := cmd.CombinedOutput()
	if err == nil {
		return HEALTHY, nil
	}

	log.Printf("Error running command: %s (%s)\n", err.Error(), output)
	return SICKLY, err
}
