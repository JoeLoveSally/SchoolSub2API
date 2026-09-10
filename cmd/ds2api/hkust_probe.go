package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"ds2api/internal/hkust"
)

const hkustProbeOutputPrefix = "JOEJOEPROXY_PROBE_JSON="

type hkustProbeEnvelope struct {
	Results []hkust.ModelProbeResult `json:"results,omitempty"`
	Error   string                   `json:"error,omitempty"`
}

func runHKUSTProbeCLI(args []string) int {
	flags := flag.NewFlagSet("hkust-probe", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	timeout := flags.Duration("timeout", 10*time.Second, "timeout for each HKUST model probe")
	if err := flags.Parse(args); err != nil {
		writeHKUSTProbeEnvelope(hkustProbeEnvelope{Error: err.Error()})
		return 2
	}
	if *timeout <= 0 || *timeout > 30*time.Second {
		writeHKUSTProbeEnvelope(hkustProbeEnvelope{Error: "timeout must be greater than 0 and at most 30s"})
		return 2
	}

	cfg, enabled, err := hkust.LoadConfigFromEnv()
	if err != nil {
		writeHKUSTProbeEnvelope(hkustProbeEnvelope{Error: err.Error()})
		return 2
	}
	if !enabled {
		writeHKUSTProbeEnvelope(hkustProbeEnvelope{Error: "HKUST_TOKEN and HKUST_USE_API are required"})
		return 2
	}

	models := flags.Args()
	if len(models) == 0 {
		models = hkust.SupportedProbeModelIDs()
	}
	for i := range models {
		models[i] = strings.TrimSpace(models[i])
	}

	client := hkust.NewClient(cfg)
	results := client.ProbeModels(context.Background(), models, *timeout)
	writeHKUSTProbeEnvelope(hkustProbeEnvelope{Results: results})
	return 0
}

func writeHKUSTProbeEnvelope(envelope hkustProbeEnvelope) {
	data, err := json.Marshal(envelope)
	if err != nil {
		fmt.Fprintf(os.Stdout, "%s%s\n", hkustProbeOutputPrefix, base64.StdEncoding.EncodeToString([]byte(`{"error":"unable to encode probe result"}`)))
		return
	}
	fmt.Fprintf(os.Stdout, "%s%s\n", hkustProbeOutputPrefix, base64.StdEncoding.EncodeToString(data))
}
