package main

import (
	"context"
	"testing"

	"github.com/ONSdigital/dis-web-mount-check/config"
	"github.com/smartystreets/goconvey/convey"
)

func TestDeploymentChecker_check_BDD(t *testing.T) {
	convey.Convey("Given a DeploymentChecker with one app and Slack enabled (SlackTest=false)", t, func() {
		ctx := context.Background()
		cfg := &config.Config{
			AppsToCheck:  []string{"app1"},
			SlackEnabled: true,
			SlackTest:    false,
		}

		// Initialise once so they're never nil
		mockDep := &DeploymentStateGetterMock{}
		mockNotifier := &SlackNotifierMock{}
		dc := checkerNew(cfg, mockDep, mockNotifier)

		convey.Reset(func() { // called after the first Convey() below
			mockDep := &DeploymentStateGetterMock{}
			mockNotifier := &SlackNotifierMock{}
			dc = checkerNew(cfg, mockDep, mockNotifier)
		})

		callCheck := func(times int) {
			for i := 0; i < times; i++ {
				dc.check(ctx)
			}
		}

		convey.Convey("When the deploymentState returns the same OK state fewer than threshold times, no Slack notification is sent", func() {
			mockDep.DeploymentStateFunc = func(ctx context.Context, jobID string) (DeploymentState, error) {
				return DeploymentOK, nil
			}

			callCheck(EffectiveFilterThreshold - 1)

			convey.So(len(mockNotifier.NotifyCalls()), convey.ShouldEqual, 0)
		})

		convey.Convey("When the deploymentState returns OK enough times, Slack is notified with state=true", func() {
			mockDep.DeploymentStateFunc = func(ctx context.Context, jobID string) (DeploymentState, error) {
				return DeploymentOK, nil
			}

			callCheck(EffectiveFilterThreshold)

			convey.So(len(mockNotifier.NotifyCalls()), convey.ShouldEqual, 1)
			convey.So(mockNotifier.NotifyCalls()[0].State, convey.ShouldBeTrue)
		})

		convey.Convey("When the deploymentState returns a failing state enough times, Slack is notified with state=false", func() {
			mockDep.DeploymentStateFunc = func(ctx context.Context, jobID string) (DeploymentState, error) {
				return DeploymentNoAllocations, nil
			}

			callCheck(EffectiveFilterThreshold)

			convey.So(len(mockNotifier.NotifyCalls()), convey.ShouldEqual, 1)
			convey.So(mockNotifier.NotifyCalls()[0].State, convey.ShouldBeFalse)
		})

		convey.Convey("When the deploymentState returns a NomadProblem enough times, Slack is notified with state=false", func() {
			mockDep.DeploymentStateFunc = func(ctx context.Context, jobID string) (DeploymentState, error) {
				return DeploymentNomadProblem, nil
			}

			callCheck(EffectiveFilterThreshold)

			convey.So(len(mockNotifier.NotifyCalls()), convey.ShouldEqual, 1)
			convey.So(mockNotifier.NotifyCalls()[0].State, convey.ShouldBeFalse)
		})

		convey.Convey("When state transitions from OK -> FAIL after enough samples, both notifications are sent in order", func() {
			call := 0
			mockDep.DeploymentStateFunc = func(ctx context.Context, jobID string) (DeploymentState, error) {
				call++
				if call <= EffectiveFilterThreshold {
					return DeploymentOK, nil
				}
				return DeploymentNoAllocations, nil
			}

			// first -> OK latch
			callCheck(EffectiveFilterThreshold)

			// next -> FAIL latch
			callCheck(EffectiveFilterThreshold)

			convey.So(len(mockNotifier.NotifyCalls()), convey.ShouldEqual, 2)
			convey.So(mockNotifier.NotifyCalls()[0].State, convey.ShouldBeTrue)  // OK
			convey.So(mockNotifier.NotifyCalls()[1].State, convey.ShouldBeFalse) // FAIL
		})

		convey.Convey("If deploymentState returns DeploymentIncomplete, no Slack notification is sent", func() {
			// Step 1: Put the system into OK state using the normal path
			mockDep.DeploymentStateFunc = func(ctx context.Context, jobID string) (DeploymentState, error) {
				return DeploymentOK, nil
			}
			callCheck(EffectiveFilterThreshold) // fills the state machine with OK

			// Clear any Slack notifications triggered by the initial OK state
			mockNotifier := &SlackNotifierMock{}

			// Step 2: Now simulate DeploymentIncomplete
			mockDep.DeploymentStateFunc = func(ctx context.Context, jobID string) (DeploymentState, error) {
				return DeploymentIncomplete, nil
			}

			// Act: call the checker function again
			callCheck(1)

			// Assert: no Slack notification was sent
			convey.So(len(mockNotifier.NotifyCalls()), convey.ShouldEqual, 0)
		})
	})
}
