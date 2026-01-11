package main

import (
	"context"
	"errors"
	"flag"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"

	"github.com/manisharma/universal-log-streamer/internal"
	"github.com/rs/zerolog"
)

var Build = "v0.0.0"

func main() {
	var (
		config      internal.Config
		ctx, cancel = context.WithCancel(context.Background())
		logger      = zerolog.New(os.Stdout).Level(zerolog.DebugLevel).With().Str("app", "ulp").Str("build", Build).Timestamp().Logger()
	)
	defer cancel()

	flag.Var(&config.NamespacesToInclude, "namespacesToInclude", "kubernetes namespace to include pods from during streaming")
	flag.Var(&config.NamespacesToExclude, "namespacesToExclude", "kubernetes namespaces to exclude pods from during streaming")
	flag.Var(&config.PodLabelsToInclude, "podLabelsToInclude", "kubernetes pod labels to include pods from during streaming")
	flag.Var(&config.Keywords, "keywords", "keywords to filter logs")
	flag.StringVar(&config.TargetURLWithHostAndScheme, "targetURLWithHostAndScheme", "https://dev.api.manifestit.tech/curated_log_streamer", "target URL with host and scheme to send logs to")
	flag.StringVar(&config.Operator, "operator", "or", "operator to use for filtering logs, eg: 'or', 'and'")
	flag.IntVar(&config.BatchSize, "batchSize", 10, "count of entries to be streamed over http")
	flag.StringVar(&config.ConfigurationId, "configurationId", "", "provider configuration id for authorization")
	flag.StringVar(&config.OgranisationId, "ogranisationId", "", "ogranisation id for authorization")
	flag.StringVar(&config.SubscriptionId, "subscriptionId", "", "subscription id for authorization")
	flag.StringVar(&config.EncryptionKey, "encryptionKey", "", "encryption key to encrypt the payload")
	flag.StringVar(&config.AuthToken, "authToken", "", "token for authentication of http request")
	flag.Parse()

	if config.Keywords.Len() == 0 {
		logger.Fatal().Msg("nothing to look for in logs")
	}

	if config.Operator != "or" && config.Operator != "and" {
		logger.Fatal().Msg("invalid operator, only 'or' and 'and' are supported")
	}

	_, err := url.Parse(config.TargetURLWithHostAndScheme)
	if err != nil {
		logger.Fatal().Err(err).Msg("invalid target URL")
	}

	profiler := &http.Server{Addr: ":6060"}
	go func() {
		logger.Debug().Msg("profiler listening to connections http://localhost:6060")
		if err := profiler.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			logger.Error().Err(err).Msg("profiler.ListenAndServe() failed")
		}
	}()

	var (
		streamer    = internal.NewStreamer(config, logger)
		deathStream = make(chan os.Signal, 1)
	)

	logger.Debug().Msg("log streamer is streaming...")
	err = streamer.Start(ctx)
	if err != nil {
		logger.Error().Err(err).Msg("logStreamer.Start(ctx) failed")
	}

	// await interruptions
	signal.Notify(deathStream, os.Interrupt, syscall.SIGABRT, syscall.SIGTERM, syscall.SIGINT)
	<-deathStream

	// shutdown systems garcefully
	signal.Stop(deathStream)
	logger.Debug().Msg("streamer interrupted, shutting down")
	profiler.Shutdown(ctx)
	streamer.Stop()
	cancel()
}
