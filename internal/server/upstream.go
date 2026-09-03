package server

import (
	"fmt"

	"ds2api/internal/auth"
	"ds2api/internal/config"
	dsclient "ds2api/internal/deepseek/client"
	"ds2api/internal/hkust"
	"ds2api/internal/httpapi/openai/shared"
)

type apiUpstream struct {
	Store        shared.ConfigReader
	Auth         shared.AuthResolver
	DS           shared.DeepSeekCaller
	HKUSTEnabled bool
	HKUSTModel   string
}

type hkustConfigReader struct {
	shared.ConfigReader
}

func (hkustConfigReader) CurrentInputFileEnabled() bool {
	return false
}

func selectAPIUpstream(store *config.Store, resolver *auth.Resolver, dsClient *dsclient.Client) (apiUpstream, error) {
	cfg, enabled, err := hkust.LoadConfigFromEnv()
	if err != nil {
		return apiUpstream{}, fmt.Errorf("load HKUST upstream config: %w", err)
	}
	if !enabled {
		return apiUpstream{Store: store, Auth: resolver, DS: dsClient}, nil
	}
	return apiUpstream{
		Store:        hkustConfigReader{ConfigReader: store},
		Auth:         hkust.NewResolver(store),
		DS:           hkust.NewClient(cfg),
		HKUSTEnabled: true,
		HKUSTModel:   cfg.Model,
	}, nil
}
