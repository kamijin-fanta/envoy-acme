package common

import "github.com/kamijin-fanta/envoy-acme-sds/pkg/store"

type Notification struct {
	Certificates []*store.Certificates
}

type SitesConfig struct {
	Sites []*Site `yaml:"sites"`
}

type Site struct {
	Name     string   `yaml:"name"`
	Provider string   `yaml:"provider"`
	Email    string   `yaml:"email"`
	Domains  []string `yaml:"domains"`
	LegoEnv  []string `yaml:"legoenv"`
}

const PrometheusNamespace = "envoy_acme_sds"
