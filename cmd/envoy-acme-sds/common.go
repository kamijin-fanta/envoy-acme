package main

import (
	"fmt"
	"github.com/kamijin-fanta/envoy-acme-sds/pkg/store"
	"github.com/kamijin-fanta/envoy-acme-sds/pkg/store/consul_store"
	"github.com/kamijin-fanta/envoy-acme-sds/pkg/store/file_store"
	"github.com/urfave/cli/v2"
)

func MustInitStore(c *cli.Context) store.Store {
	var s store.Store
	var err error
	switch c.String("store") {
	case "file", "FILE":
		s, err = file_store.NewFileStore(c.String("store-file-base"))
		if err != nil {
			panic(err)
		}
	case "consul", "CONSUL":
		prefix := c.String("store-consul-prefix")
		if prefix == "" {
			panic(fmt.Sprintf("store-consul-prefix must not empty"))
		}
		s, err = consul_store.NewConsulStore(prefix)
		if err != nil {
			panic(err)
		}
	default:
		panic(fmt.Sprintf("known store type '%s'", c.String("store")))
	}
	return s
}
