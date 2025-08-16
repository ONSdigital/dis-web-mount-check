package main

// Check desired apps are running on both web_mount boxes and report any issues to Slack

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"syscall"

	"github.com/ONSdigital/dis-web-mount-check/checker"
	"github.com/ONSdigital/dis-web-mount-check/config"
	"github.com/ONSdigital/dis-web-mount-check/deployment"
	"github.com/ONSdigital/dp-healthcheck/healthcheck"
	dpnethttp "github.com/ONSdigital/dp-net/v3/http"
	nomad "github.com/ONSdigital/dp-nomad"
	"github.com/ONSdigital/log.go/v2/log"
	"github.com/gorilla/mux"
	githuberrors "github.com/pkg/errors"
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

	depl := deployment.New(cfg, nomadClient)
	// create checker and inject the real Slack notifier
	chk := checker.New(cfg, depl, checker.RealSlackNotifier{})

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
		chk.Run(ctx)
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
