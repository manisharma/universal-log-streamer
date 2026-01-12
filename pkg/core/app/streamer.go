package app

import (
	"context"
	"fmt"
	"net/url"

	"github.com/manisharma/universal-log-streamer/internal"
	"github.com/manisharma/universal-log-streamer/pkg/core/object"
	"github.com/rs/zerolog"
)

type Streamer struct {
	s *internal.Streamer
}

func NewStreamer(config object.Config, logger zerolog.Logger) (*internal.Streamer, error) {
	if err := validate(config); err != nil {
		return nil, err
	}
	return internal.NewStreamer(config, logger), nil
}

func (s *Streamer) Start(ctx context.Context) {
	s.s.Start(ctx)
}

func (s *Streamer) Stop() {
	s.s.Stop()
}

func validate(config object.Config) error {
	if config.Keywords.Len() == 0 {
		return fmt.Errorf("nothing to look for in logs")
	}
	if config.Operator != "or" && config.Operator != "and" {
		return fmt.Errorf("invalid operator, only 'or' and 'and' are supported")
	}
	_, err := url.Parse(config.TargetURLWithHostAndScheme)
	if err != nil {
		return fmt.Errorf("invalid target URL, err: %s", err.Error())
	}
	return nil
}
