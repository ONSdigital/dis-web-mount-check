package config

import (
	"time"

	"github.com/kelseyhightower/envconfig"
)

// Config represents service configuration for dis-web-mount-check
type Config struct {
	NomadEndpoint              string        `envconfig:"NOMAD_ENDPOINT"`       // get from dp-deployer
	NomadToken                 string        `envconfig:"NOMAD_TOKEN" json:"-"` // get from dp-deployer
	NomadCACert                string        `envconfig:"NOMAD_CA_CERT" json:"-"`
	NomadTLSSkipVerify         bool          `envconfig:"NOMAD_TLS_SKIP_VERIFY"`
	BindAddr                   string        `envconfig:"BIND_ADDR"`
	HealthcheckInterval        time.Duration `envconfig:"HEALTHCHECK_INTERVAL"`
	HealthcheckCriticalTimeout time.Duration `envconfig:"HEALTHCHECK_CRITICAL_TIMEOUT"`
	GracefulShutdownTimeout    time.Duration `envconfig:"GRACEFUL_SHUTDOWN_TIMEOUT"`
	AppsToCheck                []string      `envconfig:"APPS_TO_CHECK"`
	SlackEnabled               bool          `envconfig:"SLACK_ENABLED"`
	SlackTest                  bool          `envconfig:"SLACK_TEST"`
	SlackAPIToken              string        `envconfig:"SLACK_API_TOKEN"  json:"-"`
	SlackUserName              string        `envconfig:"SLACK_USER_NAME"`
	SlackAlarmChannel          string        `envconfig:"SLACK_ALARM_CHANNEL"`
	SlackAlarmEmoji            string        `envconfig:"SLACK_ALARM_EMOJI"`
	SlackOKEmoji               string        `envconfig:"SLACK_OK_EMOJI"`
}

var cfg *Config

// Get returns the default config with any modifications through environment
// variables
func Get() (*Config, error) {
	if cfg != nil {
		return cfg, nil
	}

	cfg = &Config{
		NomadEndpoint:              "http://localhost:4646",
		NomadToken:                 "",
		NomadCACert:                "",
		NomadTLSSkipVerify:         false,
		BindAddr:                   ":24310",
		HealthcheckInterval:        time.Second * 30,
		HealthcheckCriticalTimeout: time.Second * 10,
		GracefulShutdownTimeout:    time.Second * 5,
		AppsToCheck:                []string{"babbage", "zebedee-reader", "the-train", "elasticsearch"},
		SlackEnabled:               false,
		SlackTest:                  false,
		SlackAPIToken:              "", // is retrieved from env
		SlackUserName:              "Spread Check",
		SlackAlarmChannel:          "#sandbox-alarm",
		SlackAlarmEmoji:            ":x:",                // a red cross
		SlackOKEmoji:               ":white_check_mark:", // a white tick in green box
	}
	return cfg, envconfig.Process("", cfg)
}
