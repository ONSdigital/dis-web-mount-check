package deployment

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ONSdigital/dis-web-mount-check/config"
	nomad "github.com/ONSdigital/dp-nomad"
	"github.com/ONSdigital/log.go/v2/log"
	"github.com/hashicorp/nomad/api"
	"github.com/hashicorp/nomad/nomad/structs"
)

const allocationsURL = "%s/v1/job/%s/allocations"

// Deployment represents a deployment.
type Deployment struct {
	nomadClient  *nomad.Client
	endpoint     string
	token        string
	notifierTest bool

	// getFunc is optional, used by tests to override network calls.
	// If nil, the normal get() implementation is used.
	getFunc func(ctx context.Context, url string, v interface{}) error
}

// New returns a new deployment.
func New(cfg *config.Config, nomadClient *nomad.Client) *Deployment {
	return &Deployment{
		nomadClient:  nomadClient,
		endpoint:     cfg.NomadEndpoint,
		token:        cfg.NomadToken,
		notifierTest: cfg.SlackTest,
	}
}

type AppInfo struct {
	Name     string // nomad client response looks like, eg: "babbage.web[1]"
	NodeName string // nomad client response looks like, eg: "ip-10-30-140-51"
}

// DeploymentState - iota enum of possible deployment states
type DeploymentState int

// Possible values for a State of a deployment. It can only be one of the following:
const (
	DeploymentUnknown                DeploymentState = iota // Startup state is unknown, not a failure
	DeploymentNomadProblem                                  // This is a failure
	DeploymentNoAllocations                                 // This is a failure
	DeploymentLessThanTwoAllocations                        // This is a failure
	DeploymentNotSpreadOverTwoBoxes                         // This is a failure
	DeploymentOK                                            // All is OK, not a failure
	DeploymentIncomplete                                    // Interim state during deployment, not a failure
)

var testCount int

// deploymentState checks spread of app (derived from dp-deployer - successCheckByAllocations() )
func (d *Deployment) DeploymentState(ctx context.Context, jobID string, sequenceCount int) (DeploymentState, error) {
	if d.notifierTest {
		// this section is for exercising and observing notifier messages working
		testCount++
		if testCount <= 6*sequenceCount {
			fmt.Printf("test count: %d\n", testCount)
		}
		if testCount <= sequenceCount {
			// Wait till enough filterng checks have resulted in reporting apps OK,
			return DeploymentOK, nil
		}
		if testCount > sequenceCount && testCount <= (4*sequenceCount) {
			// then cause report of a fail after enough fail samples have been counted,
			return DeploymentNoAllocations, nil
		}
		// then resume reporting actual real app states.
	}

	minLogData := log.Data{"job": jobID}

	var allocations []api.AllocationListStub
	if err := d.get(ctx, fmt.Sprintf(allocationsURL, d.endpoint, jobID), &allocations); err != nil {
		return DeploymentNomadProblem, err
	}

	if len(allocations) == 0 {
		log.Warn(ctx, "deployment issue - No allocations", minLogData)
		return DeploymentNoAllocations, nil
	}

	desiredStopIsRunning := false
	var desiredAllocations []api.AllocationListStub
	var allAppInfo []AppInfo
	for i := range allocations {
		allocation := &allocations[i] // pointer to avoid struct copy
		if allocation.DesiredStatus == structs.AllocDesiredStatusRun {
			desiredAllocations = append(desiredAllocations, *allocation)
			// accumulate desired app info
			if strings.Contains(allocation.Name, ".web[") {
				// only store info for web mount boxes
				allAppInfo = append(allAppInfo, AppInfo{
					Name:     allocation.Name,
					NodeName: allocation.NodeName,
				})
			}
		} else if allocation.DesiredStatus != structs.AllocDesiredStatusRun &&
			allocation.ClientStatus == structs.AllocClientStatusRunning {
			desiredStopIsRunning = true
			break
		}
	}

	if !desiredStopIsRunning {
		updatedAllocations := 0
		for i := range desiredAllocations {
			allocation := &desiredAllocations[i]
			if allocation.ClientStatus == structs.AllocClientStatusRunning {
				updatedAllocations++
			}
		}

		if len(desiredAllocations) == updatedAllocations {
			if len(allAppInfo) < 2 {
				log.Warn(ctx, "deployment issue - Less than 2 deployments", minLogData)
				return DeploymentLessThanTwoAllocations, nil
			}

			// now deduce counts of distinct NodeName's
			counts := make(map[string]int)
			for _, info := range allAppInfo {
				counts[info.NodeName]++
			}

			// there should be two web mount boxes, each with a deployment of the app being checked
			if len(counts) < 2 {
				log.Warn(ctx, "deployment issue - Not spread over 2 boxes", minLogData)
				return DeploymentNotSpreadOverTwoBoxes, nil
			}
			return DeploymentOK, nil
		}
	}
	// Deployments can take a while to complete, so we consider things as ok for now,
	// besides deployment failures are looked after by developers and other slack notifications
	log.Warn(ctx, "deployment incomplete - will re-check", minLogData)
	return DeploymentIncomplete, nil
}

// get - typically runs very quickly, but we catch any issues with use of context
// testability note: if d.getFunc != nil it will be used (tests set this).
func (d *Deployment) get(ctx context.Context, url string, v interface{}) error {
	if d.getFunc != nil {
		return d.getFunc(ctx, url, v)
	}

	var err error
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", url, http.NoBody)
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Nomad-Token", d.token)

	res, err := d.nomadClient.Client.Do(ctx, req)
	if err != nil {
		return err
	}

	defer func() {
		if cerr := res.Body.Close(); cerr != nil && err == nil {
			err = ctx.Err()
		}
	}()

	b, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}
	if res.StatusCode != http.StatusOK {
		return errors.New("unexpected response from client, Body: " + string(b) +
			" StatusCode: " + strconv.Itoa(res.StatusCode) +
			" URL: " + res.Request.URL.String())
	}
	if err := json.Unmarshal(b, v); err != nil {
		return err
	}
	return nil
}
