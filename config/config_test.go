package config

import (
	"os"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSpec(t *testing.T) {
	Convey("Given an environment with no environment variables set", t, func() {
		os.Clearenv()

		Convey("When the config is retrieved", func() {
			cfg, err := Get()
			Convey("Then there should be no error returned", func() {
				So(err, ShouldBeNil)
			})

			Convey("The values should be set to the expected defaults", func() {
				So(cfg.NomadEndpoint, ShouldEqual, "http://localhost:4646")
				So(cfg.NomadToken, ShouldEqual, "")
				So(cfg.NomadCACert, ShouldEqual, "")
				So(cfg.NomadTLSSkipVerify, ShouldBeFalse)
				So(cfg.BindAddr, ShouldEqual, ":24310")
				So(cfg.HealthcheckInterval, ShouldEqual, time.Second*30)
				So(cfg.HealthcheckCriticalTimeout, ShouldEqual, time.Second*10)
				So(cfg.GracefulShutdownTimeout, ShouldEqual, 5*time.Second)
				So(cfg.AppsToCheck, ShouldEqual, []string{"babbage", "zebedee-reader", "the-train", "elasticsearch"})
				So(cfg.SlackEnabled, ShouldEqual, false)
				So(cfg.SlackTest, ShouldEqual, false)
				So(cfg.SlackAPIToken, ShouldEqual, "")
				So(cfg.SlackUserName, ShouldEqual, "Spread Check")
				So(cfg.SlackAlarmChannel, ShouldEqual, "#sandbox-alarm")
				So(cfg.SlackAlarmEmoji, ShouldEqual, ":x:")
				So(cfg.SlackOKEmoji, ShouldEqual, ":white_check_mark:")
			})
		})
	})
}
