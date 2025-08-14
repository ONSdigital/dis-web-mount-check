# dis-web-mount-check

Once a minute, checks for the expected apps in the web-mount boxes and reports any change of state to slack.

## Getting started

* Run `make debug` to run application on http://localhost:24310
* Run `make help` to see full list of make targets

### Dependencies

* No further dependencies other than those defined in `go.mod`

### Configuration

<!-- markdownlint-disable MD034 -->

| Environment variable         | Default                        | Description                                                                                          |
| ---------------------------- | ------------------------------ | -----------------------------------------------------------------------------------------------------|
| NOMAD_ENDPOINT               | http://localhost:4646          | The endpoint of the Nomad API                                                                        |
| NOMAD_TOKEN                  |                                | The ACL token used to authorise HTTP requests                                                        |
| NOMAD_CA_CERT                |                                | The path to the CA cert file                                                                         |
| NOMAD_TLS_SKIP_VERIFY        | false                          | When using TLS to nomad, skip checking certs                                                         |
| BIND_ADDR                    | :24310                         | The listen address to bind to                                                                        |
| HEALTHCHECK_INTERVAL         | 10s                            | The time between calling healthcheck endpoints for check subsystems                                  |
| HEALTHCHECK_CRITICAL_TIMEOUT | 60s                            | The time taken for the health changes from warning state to critical due to subsystem check failures |
| GRACEFUL_SHUTDOWN_TIMEOUT    | 5s                             | Time time to wait when gracefully shutting down before closing                                       |
| APPS_TO_CHECK                | []string{"app1", "app2", etc } | List of apps expected on each web-mount box                                                          |
| SLACK_ENABLED                | false                          | Whether to send a slack message on failed checks                                                     |
| SLACK_TEST                   | false                          | Used to force alerts at app startup to confirm slack comm's working                                  |
| SLACK_API_TOKEN              | "get from env"                 | A valid slack api token (suppressed from logs)                                                       |
| SLACK_USER_NAME              | "Spread Check"                 | User name to be used for slack messages                                                              |
| SLACK_ALARM_CHANNEL          | "#sandbox-alarm"               | Slack channel to send alarm messages to                                                              |
| SLACK_ALARM_EMOJI            | ":x:"                          | Emoji to use for alarm messages, a red cross                                                         |
| SLACK_OK_EMOJI               | ":white_check_mark:"           | Emoji to use for alarm messages, a white tick in a green box                                         |

A valid Slack token with `chat:write` and `chat:write.customize` permissions is required if Slack notification is to be
enabled.

### Healthcheck

 The `/health` endpoint returns the current status of the service. Dependent services are health checked on an interval defined by the `HEALTHCHECK_INTERVAL` environment variable.

On a development machine, a request to the health check endpoint can be made by:

`curl localhost:24300/health`

### How to test the dis-web-mount-check in the environment (outside of concourse)

You will need to get the secrets: NOMAD_TOKEN and SLACK_API_TOKEN from vault.

Set up your AWS access to Sandbox in the usual way.

Make any changes you want to the app for testing ...

If this app is already running via nomad and concourse, stop it in nomad and pause its pipeline in concourse.

Then run:

```bash
make buildamd64
make pushamd64
```

Then log into web 3 box in sandbox:

```bash
dp ssh sandbox web 3
```

Then:

```bash
cd test
```

Then do:

```bash
export NOMAD_TLS_SKIP_VERIFY=true
export NOMAD_ENDPOINT=https://localhost:4646
export SLACK_ENABLED=true
```

Then run the following two lines with a SPACE at the start of the line to not save the secret in bash history:

```bash
 export NOMAD_TOKEN=<from vault for the app>
 export SLACK_API_TOKEN=<from vault for the app>
```

Then run the app with:

```bash
./dis-web-mount-check
```

Remember to use `CTRL-C` to stop the application after your tests.

You will also have to stop the application before doing any `make pushamd64` of any new builds to test.

If this app was already running via nomad and concourse, unpause its pipeline in concourse and start it in nomad.

## Contributing

See [CONTRIBUTING](CONTRIBUTING.md) for details.

## License

Copyright © 2025, Office for National Statistics (<https://www.ons.gov.uk>)

Released under MIT license, see [LICENSE](LICENSE.md) for details.
