# dis-web-mount-check

Check desired apps are running on both web_mount boxes and report any issues to Slack

## Getting started

* Run `make debug` to run application on http://localhost:24310
* Run `make help` to see full list of make targets

### Dependencies

* No further dependencies other than those defined in `go.mod`

### Configuration

| Environment variable         | Default            | Description
| ---------------------------- | ------------------ | -----------
| BIND_ADDR                    | :24310         | The host and port to bind to
| GRACEFUL_SHUTDOWN_TIMEOUT    | 5s                 | The graceful shutdown timeout in seconds (`time.Duration` format)
| HEALTHCHECK_INTERVAL         | 30s                | Time between self-healthchecks (`time.Duration` format)
| HEALTHCHECK_CRITICAL_TIMEOUT | 90s                | Time to wait until an unhealthy dependent propagates its state to make this app unhealthy (`time.Duration` format)
| OTEL_EXPORTER_OTLP_ENDPOINT  | localhost:4317     | Endpoint for OpenTelemetry service
| OTEL_SERVICE_NAME            | dis-web-mount-check          | Label of service for OpenTelemetry service
| OTEL_BATCH_TIMEOUT           | 5s                 | Timeout for OpenTelemetry
| OTEL_ENABLED                 | false              | Feature flag to enable OpenTelemetry

## Contributing

See [CONTRIBUTING](CONTRIBUTING.md) for details.

## License

Copyright © 2025, Office for National Statistics (<https://www.ons.gov.uk>)

Released under MIT license, see [LICENSE](LICENSE.md) for details.
