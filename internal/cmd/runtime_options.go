package cmd

import (
	"io"
	"os"
	"time"

	"github.com/gl0bal01/omi/internal/api"
)

type runtimeOptions struct {
	verbose         bool
	explainDefaults bool
	debugHTTP       bool
	apiKey          string
	timeout         time.Duration
	debugOut        io.Writer
}

var runOpts = &runtimeOptions{debugOut: os.Stderr}

func newAPIClient(apiKey string) *api.Client {
	c := api.NewClient(apiKey, "", runOpts.timeout)
	if runOpts.debugHTTP {
		c.SetDebug(true, runOpts.debugOut)
	}
	return c
}
