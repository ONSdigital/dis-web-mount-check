package main

//go:generate moq -stub -out mocks_test.go . SlackNotifier DeploymentStateGetter

// Check desired apps are running on both web_mount boxes and report any issues to Slack

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"errors"
	"net/http"

	"github.com/ONSdigital/dis-web-mount-check/config"
	"github.com/ONSdigital/dp-healthcheck/healthcheck"
	dpnethttp "github.com/ONSdigital/dp-net/v3/http"
	nomad "github.com/ONSdigital/dp-nomad"
	"github.com/ONSdigital/log.go/v2/log"
	"github.com/gorilla/mux"
	"github.com/hashicorp/nomad/api"
	"github.com/hashicorp/nomad/nomad/structs"
	githuberrors "github.com/pkg/errors"
	"github.com/slack-go/slack"
)

var (
	// BuildTime represents the time in which the service was built
	BuildTime string
	// GitCommit represents the commit (SHA-1) hash of the service that is running
	GitCommit string
	// Version represents the version of the service that is running
	Version string // used in 'healthcheck.NewVersionInfo()'
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	log.Namespace = "dis-web-mount-check"

	cfg, err := config.Get()
	if err != nil {
		log.Fatal(ctx, "Failed to initialise config", err)
		os.Exit(1)
	}

	log.Info(ctx, "config on startup", log.Data{"config": cfg, "build_time": BuildTime, "git-commit": GitCommit})

	// create Nomad client
	nomadClient, err := nomad.NewClient(cfg.NomadEndpoint, cfg.NomadCACert, cfg.NomadTLSSkipVerify)
	if err != nil || nomadClient == nil {
		log.Fatal(ctx, "error creating nomad client", err)
		os.Exit(1)
	}

	deployment := deploymentNew(cfg, nomadClient)
	// create checker and inject the real Slack notifier
	checker := checkerNew(cfg, deployment, RealSlackNotifier{})

	hc, err := startHealthChecks(ctx, cfg, nomadClient)
	if err != nil {
		log.Fatal(ctx, "failed to start healthchecks", err)
		os.Exit(1)
	}

	r := mux.NewRouter()
	r.HandleFunc("/health", hc.Handler)

	sigC := make(chan os.Signal, 1)
	signal.Notify(sigC, syscall.SIGHUP, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGTERM)

	go func() {
		checker.run(ctx)
	}()

	// Create and start http server for healthcheck
	httpServer := dpnethttp.NewServer(cfg.BindAddr, r)
	go func() {
		if err := httpServer.ListenAndServe(); err != nil {
			log.Error(ctx, "error starting http server", err)
			cancel()
		}
	}()

	select {
	case sig := <-sigC:
		log.Error(ctx, "received exit signal", githuberrors.New("received exit signal"), log.Data{"signal": sig})
		cancel()
	case <-ctx.Done():
		log.Info(ctx, "context done")
	}

	log.Info(ctx, "shutdown with timeout:", log.Data{"Timeout": cfg.GracefulShutdownTimeout})
	shutdownContext, shutdownCtxCancel := context.WithTimeout(context.Background(), cfg.GracefulShutdownTimeout)

	go func() {
		// Shutdown HTTP server
		log.Info(shutdownContext, "closing http server")
		if err := httpServer.Shutdown(ctx); err != nil {
			log.Error(shutdownContext, "failed to gracefully close http server", err)
		}
		log.Info(ctx, "http server gracefully closed ")

		// Stop healthcheck
		hc.Stop()

		shutdownCtxCancel()
	}()

	<-shutdownContext.Done()
	if errors.Is(shutdownContext.Err(), context.DeadlineExceeded) {
		log.Error(shutdownContext, "shutdown timeout", shutdownContext.Err())
		os.Exit(1)
	}
	log.Error(shutdownContext, "done shutdown gracefully", errors.New("done shutdown gracefully"), log.Data{"context": shutdownContext.Err()})
}

func startHealthChecks(ctx context.Context, cfg *config.Config, nomadClient *nomad.Client) (*healthcheck.HealthCheck, error) {
	// Create healthcheck object with versionInfo
	versionInfo, err := healthcheck.NewVersionInfo(BuildTime, GitCommit, Version)
	if err != nil {
		return nil, githuberrors.Wrap(err, "failed to create service version information")
	}
	hc := healthcheck.New(versionInfo, cfg.HealthcheckCriticalTimeout, cfg.HealthcheckInterval)

	if err := hc.AddCheck("Nomad", nomadClient.Checker); err != nil {
		return nil, githuberrors.Wrap(err, "error adding check for nomad")
	}

	// Start healthcheck
	hc.Start(ctx)

	return &hc, nil
}

// ---------- Added interfaces & real notifier (for testability) ----------

// SlackNotifier is an interface for sending Slack notifications.
// Tests should mock this interface. The concrete implementation calls notifySlack().
type SlackNotifier interface {
	Notify(ctx context.Context, cfg *config.Config, result string, state bool)
}

// RealSlackNotifier implements SlackNotifier using the real notifySlack() function.
type RealSlackNotifier struct{}

// Notify implements SlackNotifier by calling the package-level notifySlack function.
func (RealSlackNotifier) Notify(ctx context.Context, cfg *config.Config, result string, state bool) {
	notifySlack(ctx, cfg, result, state)
}

// DeploymentStateGetter is an interface abstraction for deployment.deploymentState
// so tests can mock the state determination logic.
type DeploymentStateGetter interface {
	deploymentState(ctx context.Context, jobID string) (DeploymentState, error)
}

// from checker.go

// EffectiveState - iota enum of reported states
type EffectiveState int

// Possible values of EffectiveState that determines 'slack' reporting
const (
	EffectiveUnknown EffectiveState = iota + 1 // Startup state is unknown, not a failure
	EffectiveFail
	EffectiveOK
	EffectiveNomadFail
)

type BothStates struct {
	appName                 string
	deploymentState         DeploymentState
	effectiveState          EffectiveState
	effectiveFailCount      int
	effectiveOKCount        int
	effectiveNomadFailCount int
}

const EffectiveFilterThreshold = 3 // number of times new state must be seen in a row to filter out any noise

// DeploymentChecker represents a deployment checker.
// Modified to use DeploymentStateGetter and SlackNotifier for testability.
type DeploymentChecker struct {
	config        *config.Config
	deployment    DeploymentStateGetter
	allAppStates  *[]BothStates
	slackNotifier SlackNotifier
}

// checkerNew returns a new checker. Accepts a DeploymentStateGetter and a SlackNotifier to allow injection.
func checkerNew(cfg *config.Config, d DeploymentStateGetter, notifier SlackNotifier) *DeploymentChecker {
	allApps := make([]BothStates, len(cfg.AppsToCheck))
	for i, appName := range cfg.AppsToCheck {
		allApps[i] = BothStates{
			deploymentState:         DeploymentUnknown, // Assign initial state
			effectiveState:          EffectiveUnknown,
			effectiveFailCount:      0,
			effectiveOKCount:        0,
			effectiveNomadFailCount: 0,
			appName:                 appName,
		}
	}
	c := &DeploymentChecker{
		config:        cfg,
		deployment:    d,
		allAppStates:  &allApps,
		slackNotifier: notifier,
	}

	return c
}

func (dc *DeploymentChecker) run(ctx context.Context) {
	for {
		dc.check(ctx)
		if dc.config.SlackTest {
			time.Sleep(10000 * time.Millisecond) // 10 seconds
		} else {
			time.Sleep(60000 * time.Millisecond) // 60 seconds
		}
	}
}

func (dc *DeploymentChecker) check(ctx context.Context) {
	// check all applications in list

	for i := range *dc.allAppStates {
		app := &(*dc.allAppStates)[i]

		newDeploymentState, err := dc.deployment.deploymentState(ctx, app.appName)
		if err != nil {
			log.Error(ctx, "check(), deploymentState() error for: "+app.appName, err)
			continue
		}

		if app.deploymentState == newDeploymentState {
			continue
		}

		// Process state transitions
		newEffective := dc.determineNewEffective(app, newDeploymentState)
		if app.effectiveState == newEffective {
			continue
		}

		switch newEffective {
		case EffectiveFail:
			app.effectiveOKCount = 0
			app.effectiveNomadFailCount = 0

			app.effectiveFailCount++
			if app.effectiveFailCount >= EffectiveFilterThreshold {
				dc.latchAndNotify(ctx, app, newDeploymentState, newEffective,
					"deployment spread in web mount: FAIL",
					"checking app spread -> FAIL: "+app.appName, false)
			}

		case EffectiveOK:
			app.effectiveFailCount = 0
			app.effectiveNomadFailCount = 0

			app.effectiveOKCount++
			if app.effectiveOKCount >= EffectiveFilterThreshold {
				dc.latchAndNotify(ctx, app, newDeploymentState, newEffective,
					"deployment spread in web mount: OK",
					"checking app spread -> OK: "+app.appName, true)
			}

		case EffectiveNomadFail:
			app.effectiveFailCount = 0
			app.effectiveOKCount = 0

			app.effectiveNomadFailCount++
			if app.effectiveNomadFailCount >= EffectiveFilterThreshold {
				dc.latchAndNotify(ctx, app, newDeploymentState, newEffective,
					"deployment spread in web mount: unknown",
					"checking app spread -> Problem talking to Nomad: "+app.appName, false)
			}

		case EffectiveUnknown: // this case can never logically happen, but here to shut up goland compiler warnings of 'Probable bug'
			// but then goland highlights that this can never be reached through its 'data flow analysis' ... can't win.

			// nothing to do
		}
	}
}

func (dc *DeploymentChecker) determineNewEffective(app *BothStates, newDeploymentState DeploymentState) EffectiveState {
	switch newDeploymentState {
	case DeploymentNoAllocations, DeploymentLessThanTwoAllocations, DeploymentNotSpreadOverTwoBoxes:
		return EffectiveFail
	case DeploymentOK:
		return EffectiveOK
	case DeploymentNomadProblem:
		return EffectiveNomadFail
	case DeploymentUnknown:
		// Initial startup state, so no change
		return app.effectiveState
	case DeploymentIncomplete:
		// Set new to existing, to avoid reporting OK after a deployment that does
		// not transition from EffectiveFail to EffectiveOK
		return app.effectiveState
	default:
		return EffectiveUnknown
	}
}

func (dc *DeploymentChecker) latchAndNotify(ctx context.Context, app *BothStates, newDeploymentState DeploymentState, newEffective EffectiveState, logMsg, slackMsg string, success bool) {
	app.effectiveState = newEffective
	app.deploymentState = newDeploymentState
	log.Info(ctx, logMsg, log.Data{"job": app.appName})
	if dc.config.SlackEnabled && dc.slackNotifier != nil {
		dc.slackNotifier.Notify(ctx, dc.config, slackMsg, success)
	}
}

// from deployment.go

const (
	allocationsURL = "%s/v1/job/%s/allocations"
)

// Deployment represents a deployment.
type Deployment struct {
	nomadClient *nomad.Client
	endpoint    string
	token       string
	slackTest   bool

	// getFunc is optional, used by tests to override network calls.
	// If nil, the normal get() implementation is used.
	getFunc func(ctx context.Context, url string, v interface{}) error
}

// New returns a new deployment.
func deploymentNew(cfg *config.Config, nomadClient *nomad.Client) *Deployment {
	return &Deployment{
		nomadClient: nomadClient,
		endpoint:    cfg.NomadEndpoint,
		token:       cfg.NomadToken,
		slackTest:   cfg.SlackTest,
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
func (d *Deployment) deploymentState(ctx context.Context, jobID string) (DeploymentState, error) {
	if d.slackTest {
		testCount++
		if testCount <= 60 {
			fmt.Printf("test count: %d\n", testCount)
		}
		if testCount <= 12 {
			// Wait till 3 checks have resulted in reporting apps OK,
			return DeploymentOK, nil
		}
		if testCount > 12 && testCount <= 40 {
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

func notifySlack(ctx context.Context, cfg *config.Config, result string, state bool) {
	slackAPI := slack.New(cfg.SlackAPIToken)

	emoji := cfg.SlackAlarmEmoji
	if state {
		emoji = cfg.SlackOKEmoji
	}
	header := fmt.Sprintf("%s *'web-mount' app spread check (test - ignore):*", emoji)

	text := header + "\n" + result + "\n"

	channelID, timestamp, postErr := slackAPI.PostMessageContext(ctx, cfg.SlackAlarmChannel,
		slack.MsgOptionText(text, false),
		slack.MsgOptionUsername(cfg.SlackUserName),
	)

	if postErr != nil {
		log.Error(ctx, "failed to send Slack message", postErr)
	} else {
		log.Info(ctx, fmt.Sprintf("Slack message sent to channel %s at %s", channelID, timestamp))
	}
}
