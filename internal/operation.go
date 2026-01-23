package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/nxadm/tail"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"
)

func (s *Streamer) tail(ctx context.Context, path string) {
	// wait for some time to allow pod to be fully initialized
	time.Sleep(5 * time.Second)
	entryBase := entry{Host: s.hostname}

	if s.isK8s {
		// Path: /var/log/pods/NAMESPACE_PODNAME_UID/CONTAINER/0.log
		// eg: 	/var/log/pods/mit-runtime_runtime-aws-605-worker-65bfc994c9-kcb84_b7fecddb-34f2-404c-abad-b542fe6d5947/runtime-aws-605-worker/0.log
		parts := strings.Split(path, "/")
		if len(parts) >= 3 {
			podDir := parts[len(parts)-3]
			podParts := strings.Split(podDir, "_")
			entryBase.Namespace = podParts[0]
			entryBase.Pod = podParts[1]
			entryBase.Container = parts[len(parts)-2]
			image, imageId, curated := s.getK8sMetadata(ctx, entryBase.Namespace, entryBase.Pod, entryBase.Container)
			if !curated {
				return
			}
			entryBase.Image = image
			entryBase.ImageId = imageId
		}
	} else {
		// VM/Bare Metal: Use path and hostname as identifiers
		entryBase.Namespace = "non-k8s"
		entryBase.Pod = s.hostname
		entryBase.Container = filepath.Base(path)
		entryBase.Image = "os-native"
		entryBase.ImageId = "n/a"
		if !s.isPodCurated(&entryBase.Namespace, nil) {
			return
		}
	}

	logger := s.logger.With().
		Str("namespace", entryBase.Namespace).
		Str("pod", entryBase.Pod).
		Str("container", entryBase.Container).
		Str("path", path).
		Logger()

	t, err := tail.TailFile(path, tail.Config{
		Follow:    true,
		ReOpen:    true,
		MustExist: true,
		Location:  &tail.SeekInfo{Offset: 0, Whence: io.SeekEnd},
	})
	if err != nil {
		logger.Error().Err(err).Msgf("tailing failed")
		return
	}
	defer t.Stop()

	logger.Info().Msg("tailing started")
	defer func() {
		logger.Info().Msg("tailing aborted")
	}()

	for line := range t.Lines {
		select {
		case <-s.kill:
			t.Cleanup()
			s.removeItemFromMetadataCache(entryBase.Namespace + "/" + entryBase.Pod + "/" + entryBase.Container)
			s.flush(s.entries)
			return
		default:
		}
		entry := entryBase
		if s.isK8s {
			// CRI format: <ts> <stream> <flag> <msg>
			logParts := strings.SplitN(line.Text, " ", 4)
			if len(logParts) == 4 {
				entry.Logs = logParts[3]
			} else {
				entry.Logs = line.Text
			}
		} else {
			entry.Logs = line.Text
		}
		if s.meetsCriteria(&line.Text) {
			s.pileUpOrFlush(entry)
		}
	}
}

func (s *Streamer) meetsCriteria(line *string) bool {
	if line == nil || len(*line) == 0 {
		return false
	}
	if !s.coreFilterRegex.MatchString(*line) {
		return false
	}
	if s.cfg.Operator == "and" {
		// 4xx status code must be in conjunction with 'failed' or 'error'
		if s.regexp4xxValue != nil && s.regexp4xxValue.MatchString(*line) {
			// short circuit if either 'failed' or 'error' is present
			if s.failedRegex.MatchString(*line) || s.errorRegex.MatchString(*line) {
				return true
			}
		}
		// 5xx status code must be in conjunction with 'failed' or 'error'
		if s.regexp5xxValue != nil && s.regexp5xxValue.MatchString(*line) {
			// short circuit if either 'failed' or 'error' is present
			if s.failedRegex.MatchString(*line) || s.errorRegex.MatchString(*line) {
				return true
			}
		}
		// fallback all filters must match
		for _, filter := range s.filtersRegex {
			if !filter.MatchString(*line) {
				return false
			}
		}
		return true
	}
	// 4xx status code must be in conjunction with 'failed' or 'error'
	if s.regexp4xxValue != nil && s.regexp4xxValue.MatchString(*line) {
		// short circuit if either 'failed' or 'error' is present
		if s.failedRegex.MatchString(*line) || s.errorRegex.MatchString(*line) {
			return true
		}
	}
	// 5xx status code must be in conjunction with 'failed' or 'error'
	if s.regexp5xxValue != nil && s.regexp5xxValue.MatchString(*line) {
		// short circuit if either 'failed' or 'error' is present
		if s.failedRegex.MatchString(*line) || s.errorRegex.MatchString(*line) {
			return true
		}
	}
	for _, filter := range s.filtersRegex {
		if filter.MatchString(*line) {
			return true
		}
	}
	return false
}

func (s *Streamer) removeItemFromMetadataCache(key string) {
	s.metadataCache.Delete(key)
}

func (s *Streamer) getK8sMetadata(ctx context.Context, ns, pod, container string) (string, string, bool) {
	key := ns + "/" + pod + "/" + container
	if val, ok := s.metadataCache.Load(key); ok {
		m, _ := val.([]string)
		if len(m) >= 2 {
			return m[0], m[1], true
		}
	}
	// just in case
	p, err := s.clientset.CoreV1().Pods(ns).Get(ctx, pod, v1.GetOptions{})
	if err != nil {
		s.logger.Error().Err(err).Msg("s.clientset.CoreV1().Pods(ns).Get() failed")
		return "unknown", "unknown", false
	}
	if !s.isPodCurated(&ns, p.Labels) {
		return "unknown", "unknown", false
	}
	for _, c := range p.Status.ContainerStatuses {
		if c.Name == container {
			s.metadataCache.Store(key, []string{c.Image, c.ImageID})
			return c.Image, c.ImageID, true
		}
	}
	return "unknown", "unknown", false
}

func (s *Streamer) isPodCurated(namespace *string, labels map[string]string) bool {
	if namespace != nil && len(s.cfg.NamespacesToExclude) > 0 {
		if slices.Contains(s.cfg.NamespacesToExclude, *namespace) {
			return false
		}
	}
	if namespace != nil && len(s.cfg.NamespacesToInclude) > 0 {
		if !slices.Contains(s.cfg.NamespacesToInclude, *namespace) {
			return false
		}
	}
	if len(labels) > 0 && len(s.cfg.PodLabelsToInclude) > 0 {
		for key, value := range labels {
			if slices.Contains(s.cfg.PodLabelsToInclude, key+"="+value) {
				return true
			}
		}
		return false
	}
	return true
}

func (s *Streamer) pileUpOrFlush(logEntry entry) {
	s.locker.Lock()
	s.entries = append(s.entries, logEntry)
	shouldFlush := len(s.entries) >= s.cfg.BatchSize
	if shouldFlush {
		batchToSend := s.entries
		s.entries = s.entries[:0]
		s.locker.Unlock()
		s.flush(batchToSend)
		return
	}
	s.locker.Unlock()
}

func (s *Streamer) watchForNewPods(ctx context.Context) {
	var (
		factory  = informers.NewSharedInformerFactoryWithOptions(s.clientset, time.Minute*5)
		informer = factory.Core().V1().Pods()
	)

	informer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj any) {
			pod, ok := obj.(*corev1.Pod)
			if !ok {
				s.logger.Error().Msg("AddFunc: obj.(*corev1.Pod) failed")
				return
			}
			if pod == nil || !s.isPodCurated(&pod.Namespace, pod.Labels) {
				return
			}
			for _, c := range pod.Status.ContainerStatuses {
				// Path: /var/log/pods/NAMESPACE_PODNAME_UID/CONTAINER/0.log
				logSource := fmt.Sprintf("/var/log/pods/%s_%s_%s/%s", pod.Namespace, pod.Name, string(pod.UID), c.Name)
				err := filepath.Walk(logSource, func(path string, info os.FileInfo, err error) error {
					if info != nil && !info.IsDir() && filepath.Ext(path) == ".log" {
						_, cancel := context.WithCancel(ctx)
						key := pod.Namespace + "/" + pod.Name + "/" + c.Name
						s.metadataCache.Store(key, containerInfo{
							Images:    []string{c.Image, c.ImageID},
							Namespace: pod.Namespace,
							Pod:       pod.Name,
							UID:       string(pod.UID),
							Container: c.Name,
							Cancel:    cancel,
						})
						s.logger.Debug().Str("key", key).Msg("entry added to metadataCache")
						go s.tail(ctx, path)
					}
					return nil
				})
				if err != nil {
					s.logger.Error().Err(err).
						Str("namespace", pod.Namespace).
						Str("pod", pod.Name).
						Str("container", c.Name).
						Str("logSource", logSource).
						Msg("filepath.Walk() failed")
				}
			}
		},
		DeleteFunc: func(obj interface{}) {
			pod, ok := obj.(*corev1.Pod)
			if !ok {
				s.logger.Error().Msg("DeleteFunc: obj.(*corev1.Pod) failed")
				return
			}
			for _, c := range pod.Status.ContainerStatuses {
				key := pod.Namespace + "/" + pod.Name + "/" + c.Name
				item, ok := s.metadataCache.LoadAndDelete(key)
				if ok {
					c, ok := item.(containerInfo)
					if ok && c.Cancel != nil {
						c.Cancel()
					}
					s.logger.Info().Str("key", key).Msg("entry removed from metadataCache")
				}
			}
		},
	})

	factory.Start(s.kill)
	if !cache.WaitForNamedCacheSync("logs_streamer", s.kill, informer.Informer().HasSynced) {
		err := fmt.Errorf("cache.WaitForNamedCacheSync() failed")
		s.logger.Error().Msg(err.Error())
	}

	s.logger.Info().Msg("informer cache synced, watching for new pods")

	select {
	case <-ctx.Done():
	case <-s.kill:
	}

	factory.Shutdown()
	s.logger.Info().Msg("stopped watching for new pods")
}

func (s *Streamer) flush(batchToSend []entry) {
	s.logger.Debug().Int("batchSize", len(batchToSend)).Str("targetURL", s.cfg.TargetURLWithHostAndScheme).Msg("flushing a batch")

	// TODO:
	// 1. encrypt the payload
	// 2. pass authn key

	r, w := io.Pipe()
	go func() {
		defer w.Close()
		err := json.NewEncoder(w).Encode(batchToSend)
		if err != nil {
			s.logger.Error().Err(err).Msg("json.NewEncoder(w).Encode(batchToSend) failed")
			return
		}
	}()

	req, err := http.NewRequest(http.MethodPost, s.cfg.TargetURLWithHostAndScheme, r)
	if err != nil {
		s.logger.Error().Err(err).Msg("http.NewRequest() failed")
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Mit-Subscription-ID", s.cfg.SubscriptionId)
	req.Header.Set("Mit-Org-ID", s.cfg.OgranisationId)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		s.logger.Error().Err(err).Msg("s.httpClient.Do(req) failed")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		s.logger.Error().Err(err).Int("status", resp.StatusCode).Bytes("response", body).Msg("post failed")
	}
}
