package internal

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/manisharma/universal-log-streamer/pkg/core/object"
	"github.com/rs/zerolog"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var serviceAccountTokenPath = "/var/run/secrets/kubernetes.io/serviceaccount/token"

type Streamer struct {
	cfg             object.Config
	metadataCache   sync.Map
	counter         uint64
	clientset       *kubernetes.Clientset
	isK8s           bool
	hostname        string
	keywords        [][]byte
	newLine         []byte
	failedRegex     *regexp.Regexp
	errorRegex      *regexp.Regexp
	coreFilterRegex *regexp.Regexp
	filtersRegex    []*regexp.Regexp
	regexp5xxValue  *regexp.Regexp
	regexp4xxValue  *regexp.Regexp
	entries         []entry
	locker          *sync.Mutex
	httpClient      *http.Client
	logger          zerolog.Logger
	kill            chan struct{}
}

func NewStreamer(cfg object.Config, logger zerolog.Logger) *Streamer {
	var s = new(Streamer)
	s.hostname, _ = os.Hostname()
	// does service account token exist ?
	if _, err := os.Stat(serviceAccountTokenPath); err == nil {
		s.isK8s = true
		config, _ := rest.InClusterConfig()
		s.clientset, _ = kubernetes.NewForConfig(config)
	}
	s.cfg = cfg
	s.logger = logger
	s.kill = make(chan struct{})

	s.failedRegex = regexp.MustCompile(`(?i)failed`)
	s.errorRegex = regexp.MustCompile(`(?i)error`)
	s.keywords = make([][]byte, 0, len(cfg.Keywords))
	s.filtersRegex = make([]*regexp.Regexp, 0, len(cfg.Keywords))

	var (
		has4xx           = false
		has5xx           = false
		regexp5XXValue   = `(?: 5(?:0[0-8]|1[01])[ ,]|'5(?:0[0-8]|1[01])'|"5(?:0[0-8]|1[01])"|: 5(?:0[0-8]|1[01]),?)`
		regexp4XXValue   = `(?: 4(?:0[0-9]|1[0-8]|2[1-689]|31|51)[ ,]|'4(?:0[0-9]|1[0-8]|2[1-689]|31|51)'|"4(?:0[0-9]|1[0-8]|2[1-689]|31|51)"|: 4(?:0[0-9]|1[0-8]|2[1-689]|31|51),?)`
		filteredKeywords = make([]string, 0, len(cfg.Keywords))
	)

	for _, keyword := range cfg.Keywords {
		if strings.EqualFold(keyword, "4xx") {
			has4xx = true
			s.regexp4xxValue = regexp.MustCompile(regexp4XXValue)
			continue
		}
		if strings.EqualFold(keyword, "5xx") {
			has5xx = true
			s.regexp5xxValue = regexp.MustCompile(regexp5XXValue)
			continue
		}
		filteredKeywords = append(filteredKeywords, keyword)
		s.filtersRegex = append(s.filtersRegex, regexp.MustCompile(`(?i)`+regexp.QuoteMeta(keyword)))
		s.keywords = append(s.keywords, []byte(keyword))
	}

	var regexpBuilder strings.Builder
	if has4xx || has5xx {
		// if has4xx {
		// 	s.filtersRegex = append(s.filtersRegex, s.regexp4xxValue)
		// }
		// if has5xx {
		// 	s.filtersRegex = append(s.filtersRegex, s.regexp5xxValue)
		// }
		if has4xx && has5xx {
			regexpBuilder.WriteString(regexp4XXValue + "|" + regexp5XXValue)
		}
		if !has4xx && has5xx {
			regexpBuilder.WriteString(regexp5XXValue)
		}
		if has4xx && !has5xx {
			regexpBuilder.WriteString(regexp4XXValue)
		}
	}

	if len(filteredKeywords) > 0 {
		if has4xx || has5xx {
			regexpBuilder.WriteString(`|`)
		}
		regexpBuilder.WriteString(`(?i)` + strings.Join(filteredKeywords, "|"))
	}
	s.coreFilterRegex = regexp.MustCompile(regexpBuilder.String())
	if s.cfg.BatchSize == 0 {
		s.cfg.BatchSize = 10
	}
	s.entries = make([]entry, 0, s.cfg.BatchSize)
	s.locker = &sync.Mutex{}
	s.httpClient = &http.Client{Timeout: 10 * time.Second}
	return s
}

func (s *Streamer) Start(ctx context.Context) {
	var logSource string
	if s.cfg.Path != "" {
		logSource = s.cfg.Path
	} else {
		logSource = "/var/log"
	}
	if s.isK8s {
		logSource = "/var/log/pods"
		go s.watchForNewPods(ctx, logSource)
	} else {
		s.logger.Info().Msgf("crawling log directory %s on %s", logSource, s.hostname)
		err := filepath.Walk(logSource, func(path string, info os.FileInfo, err error) error {
			if !info.IsDir() && filepath.Ext(path) == ".log" {
				go s.tail(ctx, path)
			}
			return nil
		})
		if err != nil {
			s.logger.Error().Err(err).Str("logSource", logSource).Msg("crawling log directory failed")
		}
	}
}

func (s *Streamer) Stop() {
	close(s.kill)
}
