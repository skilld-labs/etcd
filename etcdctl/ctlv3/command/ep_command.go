// Copyright 2015 The etcd Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package command

import (
	"fmt"
	"os"
	"sync"
	"time"

	v3 "github.com/skilld-labs/etcd/clientv3"
	"github.com/skilld-labs/etcd/etcdserver/api/v3rpc/rpctypes"
	"github.com/skilld-labs/etcd/pkg/flags"

	"github.com/spf13/cobra"
)

// NewEndpointCommand returns the cobra command for "endpoint".
func NewEndpointCommand() *cobra.Command {
	ec := &cobra.Command{
		Use:   "endpoint <subcommand>",
		Short: "Endpoint related commands",
	}

	ec.AddCommand(newEpHealthCommand())
	ec.AddCommand(newEpStatusCommand())

	return ec
}

func newEpHealthCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "health",
		Short: "Checks the healthiness of endpoints specified in `--endpoints` flag",
		Run:   epHealthCommandFunc,
	}

	return cmd
}

func newEpStatusCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Prints out the status of endpoints specified in `--endpoints` flag",
		Long: `When --write-out is set to simple, this command prints out comma-separated status lists for each endpoint.
The items in the lists are endpoint, ID, version, db size, is leader, raft term, raft index.
`,
		Run: epStatusCommandFunc,
	}
}

type epHealth struct {
	Ep     string `json:"endpoint"`
	Health bool   `json:"health"`
	Took   string `json:"took"`
	Error  string `json:"error,omitempty"`
}

// epHealthCommandFunc executes the "endpoint-health" command.
func epHealthCommandFunc(cmd *cobra.Command, args []string) {
	flags.SetPflagsFromEnv("ETCDCTL", cmd.InheritedFlags())
	initDisplayFromCmd(cmd)
	endpoints, err := cmd.Flags().GetStringSlice("endpoints")
	if err != nil {
		ExitWithError(ExitError, err)
	}

	sec := secureCfgFromCmd(cmd)
	dt := dialTimeoutFromCmd(cmd)
	auth := authCfgFromCmd(cmd)
	cfgs := []*v3.Config{}
	for _, ep := range endpoints {
		cfg, err := newClientCfg([]string{ep}, dt, sec, auth)
		if err != nil {
			ExitWithError(ExitBadArgs, err)
		}
		cfgs = append(cfgs, cfg)
	}

	var wg sync.WaitGroup
	hch := make(chan epHealth, len(cfgs))
	for _, cfg := range cfgs {
		wg.Add(1)
		go func(cfg *v3.Config) {
			defer wg.Done()
			ep := cfg.Endpoints[0]
			cli, err := v3.New(*cfg)
			if err != nil {
				hch <- epHealth{Ep: ep, Health: false, Error: err.Error()}
				return
			}
			st := time.Now()
			// get a random key. As long as we can get the response without an error, the
			// endpoint is health.
			ctx, cancel := commandCtx(cmd)
			_, err = cli.Get(ctx, "health")
			cancel()
			eh := epHealth{Ep: ep, Health: false, Took: time.Since(st).String()}
			// permission denied is OK since proposal goes through consensus to get it
			if err == nil || err == rpctypes.ErrPermissionDenied {
				eh.Health = true
			} else {
				eh.Error = err.Error()
			}
			hch <- eh
		}(cfg)
	}

	wg.Wait()
	close(hch)

	errs := false
	healthList := []epHealth{}
	for h := range hch {
		healthList = append(healthList, h)
		if h.Error != "" {
			errs = true
		}
	}
	display.EndpointHealth(healthList)
	if errs {
		ExitWithError(ExitError, fmt.Errorf("unhealthy cluster"))
	}
}

type epStatus struct {
	Ep   string             `json:"Endpoint"`
	Resp *v3.StatusResponse `json:"Status"`
}

func epStatusCommandFunc(cmd *cobra.Command, args []string) {
	c := mustClientFromCmd(cmd)

	statusList := []epStatus{}
	var err error
	for _, ep := range c.Endpoints() {
		ctx, cancel := commandCtx(cmd)
		resp, serr := c.Status(ctx, ep)
		cancel()
		if serr != nil {
			err = serr
			fmt.Fprintf(os.Stderr, "Failed to get the status of endpoint %s (%v)\n", ep, serr)
			continue
		}
		statusList = append(statusList, epStatus{Ep: ep, Resp: resp})
	}

	display.EndpointStatus(statusList)

	if err != nil {
		os.Exit(ExitError)
	}
}
