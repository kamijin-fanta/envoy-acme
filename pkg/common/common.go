package common

import "github.com/kamijin-fanta/envoy-acme-sds/pkg/store"

type Notification struct {
	Certificates []*store.Certificates
}
