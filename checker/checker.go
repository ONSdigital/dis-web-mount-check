package checker

//go:generate moq -stub -out mocks_test.go . SlackNotifier DeploymentStateGetter

import (
	"context"
	"time"

	"github.com/ONSdigital/dis-web-mount-check/config"
	"github.com/ONSdigital/dis-web-mount-check/deployment"
	"github.com/ONSdigital/log.go/v2/log"
)

// ---------- Interfaces & real notifier (for testability) ----------

// SlackNotifier is an interface for sending Slack notifications.
type SlackNotifier interface {
	Notify(ctx context.Context, cfg *config.Config, result string, state bool)
}

// RealSlackNotifier implements SlackNotifier using deployment.NotifySlack().
type RealSlackNotifier struct{}

// Notify calls deployment.NotifySlack.
func (RealSlackNotifier) Notify(ctx context.Context, cfg *config.Config, result string, state bool) {
	deployment.NotifySlack(ctx, cfg, result, state)
}

// DeploymentStateGetter is an interface abstraction for deployment.Deployment.
type DeploymentStateGetter interface {
	DeploymentState(ctx context.Context, jobID string) (deployment.DeploymentState, error)
}

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
	deploymentState         deployment.DeploymentState
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

// New returns a new checker. Accepts a DeploymentStateGetter and a SlackNotifier to allow injection.
func New(cfg *config.Config, d DeploymentStateGetter, notifier SlackNotifier) *DeploymentChecker {
	allApps := make([]BothStates, len(cfg.AppsToCheck))
	for i, appName := range cfg.AppsToCheck {
		allApps[i] = BothStates{
			deploymentState: deployment.DeploymentUnknown, // Assign initial state
			effectiveState:  EffectiveUnknown,
			appName:         appName,
		}
	}
	return &DeploymentChecker{
		config:        cfg,
		deployment:    d,
		allAppStates:  &allApps,
		slackNotifier: notifier,
	}
}

func (dc *DeploymentChecker) Run(ctx context.Context) {
	for {
		dc.check(ctx)
		if dc.config.SlackTest {
			time.Sleep(10 * time.Second) // 10 seconds
		} else {
			time.Sleep(60 * time.Second) // 60 seconds
		}
	}
}

// check all applications in list
func (dc *DeploymentChecker) check(ctx context.Context) {
	for i := range *dc.allAppStates {
		app := &(*dc.allAppStates)[i]

		newDeploymentState, err := dc.deployment.DeploymentState(ctx, app.appName)
		if err != nil {
			log.Error(ctx, "check(), DeploymentState() error for: "+app.appName, err)
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
			app.effectiveOKCount, app.effectiveNomadFailCount = 0, 0
			app.effectiveFailCount++
			if app.effectiveFailCount >= EffectiveFilterThreshold {
				dc.latchAndNotify(ctx, app, newDeploymentState, newEffective,
					"deployment spread in web mount: FAIL",
					"checking app spread -> FAIL: "+app.appName, false)
			}
		case EffectiveOK:
			app.effectiveFailCount, app.effectiveNomadFailCount = 0, 0
			app.effectiveOKCount++
			if app.effectiveOKCount >= EffectiveFilterThreshold {
				dc.latchAndNotify(ctx, app, newDeploymentState, newEffective,
					"deployment spread in web mount: OK",
					"checking app spread -> OK: "+app.appName, true)
			}
		case EffectiveNomadFail:
			app.effectiveFailCount, app.effectiveOKCount = 0, 0
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

func (dc *DeploymentChecker) determineNewEffective(app *BothStates, newDeploymentState deployment.DeploymentState) EffectiveState {
	switch newDeploymentState {
	case deployment.DeploymentNoAllocations,
		deployment.DeploymentLessThanTwoAllocations,
		deployment.DeploymentNotSpreadOverTwoBoxes:
		return EffectiveFail
	case deployment.DeploymentOK:
		return EffectiveOK
	case deployment.DeploymentNomadProblem:
		return EffectiveNomadFail
	case deployment.DeploymentUnknown:
		// Initial startup state, so no change
		return app.effectiveState
	case deployment.DeploymentIncomplete:
		// Set new to existing, to avoid reporting OK after a deployment that does
		// not transition from EffectiveFail to EffectiveOK
		return app.effectiveState
	default:
		return EffectiveUnknown
	}
}

func (dc *DeploymentChecker) latchAndNotify(ctx context.Context, app *BothStates, newDeploymentState deployment.DeploymentState, newEffective EffectiveState, logMsg, slackMsg string, success bool) {
	app.effectiveState = newEffective
	app.deploymentState = newDeploymentState
	log.Info(ctx, logMsg, log.Data{"job": app.appName})
	if dc.config.SlackEnabled && dc.slackNotifier != nil {
		dc.slackNotifier.Notify(ctx, dc.config, slackMsg, success)
	}
}
