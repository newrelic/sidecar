// These are types that conform to the Checker interface
// and can be assigned to a Check for watching service
// health.
package healthy

import (
	"errors"
	"net/http"
	"os/exec"
)

// A Checker that makes an HTTP get call and expects to get
// a 200-299 back as success. Anything else is considered
// a failure. The URL to hit is passed ass the args to the
// Run method.
type HttpGetCmd struct {}

func (h *HttpGetCmd) Run(args string) (int, error) {
	resp, err := http.Get(args)
	if resp == nil {
		return UNKNOWN, errors.New("No body from HTTP response!")
	}
	defer resp.Body.Close()

	if resp.StatusCode > 200 && resp.StatusCode < 300 {
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
    cmd := exec.Command(args)
    err := cmd.Run()
	if err == nil {
		return HEALTHY, nil
	}

	return SICKLY, err
}
