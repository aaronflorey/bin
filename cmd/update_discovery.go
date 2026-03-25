package cmd

import (
	"fmt"
	"sort"

	"github.com/caarlos0/log"
	"github.com/marcosnils/bin/pkg/config"
	"github.com/marcosnils/bin/pkg/providers"
)

type providerFactory func(u, provider string) (providers.Provider, error)

type availableUpdate struct {
	binary *config.Binary
	info   *updateInfo
}

func collectAvailableUpdates(bins map[string]*config.Binary, newProvider providerFactory, continueOnError bool) ([]availableUpdate, map[*config.Binary]error, error) {
	paths := make([]string, 0, len(bins))
	for p := range bins {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	updates := make([]availableUpdate, 0, len(paths))
	updateFailures := map[*config.Binary]error{}

	for _, p := range paths {
		b := bins[p]
		if b.Pinned {
			continue
		}

		provider, err := newProvider(b.URL, b.Provider)
		if err != nil {
			if continueOnError {
				updateFailures[b] = fmt.Errorf("Error while creating provider for %v: %v", b.Path, err)
				continue
			}
			return nil, nil, err
		}
		log.Debugf("Using provider '%s' for '%s'", provider.GetID(), b.URL)

		ui, err := getLatestVersion(b, provider)
		if err != nil {
			if continueOnError {
				updateFailures[b] = fmt.Errorf("Error while getting latest version of %v: %v", b.Path, err)
				continue
			}
			return nil, nil, err
		}

		if ui != nil {
			updates = append(updates, availableUpdate{
				binary: b,
				info:   ui,
			})
		}
	}

	return updates, updateFailures, nil
}
