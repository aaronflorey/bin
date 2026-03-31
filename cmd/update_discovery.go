package cmd

import (
	"fmt"
	"sort"
	"sync"

	"github.com/aaronflorey/bin/pkg/config"
	"github.com/aaronflorey/bin/pkg/providers"
	"github.com/caarlos0/log"
)

type providerFactory func(u, provider string) (providers.Provider, error)

type availableUpdate struct {
	binary *config.Binary
	info   *updateInfo
}

const defaultUpdateParallelism = 10

func collectAvailableUpdates(bins map[string]*config.Binary, newProvider providerFactory, continueOnError bool, parallelism int) ([]availableUpdate, map[*config.Binary]error, error) {
	if parallelism <= 0 {
		parallelism = defaultUpdateParallelism
	}

	paths := make([]string, 0, len(bins))
	for p := range bins {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	updates := make([]availableUpdate, 0, len(paths))
	updateFailures := map[*config.Binary]error{}

	type updateResult struct {
		path    string
		update  *availableUpdate
		binary  *config.Binary
		failure error
		err     error
	}

	jobs := make(chan string)
	results := make(chan updateResult, len(paths))

	workerCount := parallelism
	if workerCount > len(paths) {
		workerCount = len(paths)
	}
	if workerCount < 1 {
		workerCount = 1
	}

	var wg sync.WaitGroup
	for range workerCount {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for p := range jobs {
				b := bins[p]
				if b.Pinned {
					results <- updateResult{path: p}
					continue
				}

				provider, err := newProvider(b.URL, b.Provider)
				if err != nil {
					failure := fmt.Errorf("Error while creating provider for %v: %v", b.Path, err)
					if continueOnError {
						results <- updateResult{path: p, binary: b, failure: failure}
						continue
					}
					results <- updateResult{path: p, err: failure}
					continue
				}
				log.Debugf("Using provider '%s' for '%s'", provider.GetID(), b.URL)

				ui, err := getLatestVersion(b, provider)
				if err != nil {
					failure := fmt.Errorf("Error while getting latest version of %v: %v", b.Path, err)
					if continueOnError {
						results <- updateResult{path: p, binary: b, failure: failure}
						continue
					}
					results <- updateResult{path: p, err: failure}
					continue
				}

				if ui != nil {
					update := availableUpdate{binary: b, info: ui}
					results <- updateResult{path: p, update: &update}
					continue
				}

				results <- updateResult{path: p}
			}
		}()
	}

	go func() {
		for _, p := range paths {
			jobs <- p
		}
		close(jobs)
		wg.Wait()
		close(results)
	}()

	var firstErr error
	for result := range results {
		if result.update != nil {
			updates = append(updates, *result.update)
			continue
		}
		if result.failure != nil {
			updateFailures[result.binary] = result.failure
			continue
		}
		if result.err != nil && firstErr == nil {
			firstErr = result.err
		}
	}

	if firstErr != nil {
		return nil, nil, firstErr
	}

	sort.Slice(updates, func(i, j int) bool {
		return updates[i].binary.Path < updates[j].binary.Path
	})

	return updates, updateFailures, nil
}
