package deployment

import (
	"context"
	"errors"
	"testing"

	"github.com/hashicorp/nomad/api"
	"github.com/hashicorp/nomad/nomad/structs"
	"github.com/smartystreets/goconvey/convey"
)

func TestDeployment_deploymentState_BDD(t *testing.T) {
	convey.Convey("Given a Deployment (slackTest=false)", t, func() {
		ctx := context.Background()
		d := &Deployment{slackTest: false}

		convey.Convey("When get returns an error, DeploymentNomadProblem is returned", func() {
			d.getFunc = func(ctx context.Context, url string, v interface{}) error {
				return errors.New("some client error")
			}

			state, err := d.DeploymentState(ctx, "job1")
			convey.So(err, convey.ShouldNotBeNil)
			convey.So(state, convey.ShouldEqual, DeploymentNomadProblem)
		})

		convey.Convey("When no allocations, DeploymentNoAllocations is returned", func() {
			d.getFunc = func(ctx context.Context, url string, v interface{}) error {
				ptr := v.(*[]api.AllocationListStub)
				*ptr = []api.AllocationListStub{}
				return nil
			}

			state, err := d.DeploymentState(ctx, "job1")
			convey.So(err, convey.ShouldBeNil)
			convey.So(state, convey.ShouldEqual, DeploymentNoAllocations)
		})

		convey.Convey("When less than two desired web allocations, returns DeploymentLessThanTwoAllocations", func() {
			d.getFunc = func(ctx context.Context, url string, v interface{}) error {
				ptr := v.(*[]api.AllocationListStub)
				*ptr = []api.AllocationListStub{
					{
						DesiredStatus: structs.AllocDesiredStatusRun,
						ClientStatus:  structs.AllocClientStatusRunning,
						Name:          "app.web[1]",
						NodeName:      "node1",
					},
				}
				return nil
			}

			state, err := d.DeploymentState(ctx, "job1")
			convey.So(err, convey.ShouldBeNil)
			convey.So(state, convey.ShouldEqual, DeploymentLessThanTwoAllocations)
		})

		convey.Convey("When two desired allocations are present but on the same node, returns DeploymentNotSpreadOverTwoBoxes", func() {
			d.getFunc = func(ctx context.Context, url string, v interface{}) error {
				ptr := v.(*[]api.AllocationListStub)
				*ptr = []api.AllocationListStub{
					{
						DesiredStatus: structs.AllocDesiredStatusRun,
						ClientStatus:  structs.AllocClientStatusRunning,
						Name:          "app.web[1]",
						NodeName:      "node1",
					},
					{
						DesiredStatus: structs.AllocDesiredStatusRun,
						ClientStatus:  structs.AllocClientStatusRunning,
						Name:          "app.web[2]",
						NodeName:      "node1",
					},
				}
				return nil
			}

			state, err := d.DeploymentState(ctx, "job1")
			convey.So(err, convey.ShouldBeNil)
			convey.So(state, convey.ShouldEqual, DeploymentNotSpreadOverTwoBoxes)
		})

		convey.Convey("When two desired allocations are present on two different nodes, returns DeploymentOK", func() {
			d.getFunc = func(ctx context.Context, url string, v interface{}) error {
				ptr := v.(*[]api.AllocationListStub)
				*ptr = []api.AllocationListStub{
					{
						DesiredStatus: structs.AllocDesiredStatusRun,
						ClientStatus:  structs.AllocClientStatusRunning,
						Name:          "app.web[1]",
						NodeName:      "node1",
					},
					{
						DesiredStatus: structs.AllocDesiredStatusRun,
						ClientStatus:  structs.AllocClientStatusRunning,
						Name:          "app.web[2]",
						NodeName:      "node2",
					},
				}
				return nil
			}

			state, err := d.DeploymentState(ctx, "job1")
			convey.So(err, convey.ShouldBeNil)
			convey.So(state, convey.ShouldEqual, DeploymentOK)
		})

		convey.Convey("When a non-desired 'stop' allocation is running, returns DeploymentIncomplete", func() {
			d.getFunc = func(ctx context.Context, url string, v interface{}) error {
				ptr := v.(*[]api.AllocationListStub)
				*ptr = []api.AllocationListStub{
					{
						DesiredStatus: "stop",
						ClientStatus:  structs.AllocClientStatusRunning,
						Name:          "app.web[1]",
						NodeName:      "node1",
					},
				}
				return nil
			}

			state, err := d.DeploymentState(ctx, "job1")
			convey.So(err, convey.ShouldBeNil)
			convey.So(state, convey.ShouldEqual, DeploymentIncomplete)
		})
	})
}
