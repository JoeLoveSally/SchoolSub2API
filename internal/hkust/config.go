package hkust

import (
	"fmt"
	"os"
	"strings"
)

const (
	defaultEndpoint = "wss://aigc.hkust-gz.edu.cn/chat/new"
	defaultOrigin   = "https://aigc.hkust-gz.edu.cn"
	defaultModel    = "DeepSeek-V4-Pro-conv"
)

type Config struct {
	Endpoint string
	Origin   string
	Token    string
	UseAPI   string
	Model    string
}

func LoadConfigFromEnv() (Config, bool, error) {
	token := strings.TrimSpace(os.Getenv("HKUST_TOKEN"))
	useAPI := strings.TrimSpace(os.Getenv("HKUST_USE_API"))
	endpoint := strings.TrimSpace(os.Getenv("HKUST_WS_URL"))

	enabled := token != "" || useAPI != "" || endpoint != ""
	if !enabled {
		return Config{}, false, nil
	}
	if token == "" {
		return Config{}, true, fmt.Errorf("HKUST_TOKEN is required when HKUST upstream is enabled")
	}
	if useAPI == "" {
		return Config{}, true, fmt.Errorf("HKUST_USE_API is required when HKUST upstream is enabled")
	}
	if endpoint == "" {
		endpoint = defaultEndpoint
	}
	origin := strings.TrimSpace(os.Getenv("HKUST_ORIGIN"))
	if origin == "" {
		origin = defaultOrigin
	}
	model := strings.TrimSpace(os.Getenv("HKUST_MODEL"))
	if model == "" {
		model = defaultModel
	}

	return Config{
		Endpoint: endpoint,
		Origin:   origin,
		Token:    token,
		UseAPI:   useAPI,
		Model:    model,
	}, true, nil
}
