package main

import (
	"context"
	"github.com/ghodss/yaml"
	"github.com/kamijin-fanta/envoy-acme-sds/pkg/acme_service"
	"github.com/kamijin-fanta/envoy-acme-sds/pkg/common"
	"github.com/kamijin-fanta/envoy-acme-sds/pkg/xds_service"
	"github.com/rs/xid"
	"github.com/urfave/cli/v2"
	"io/ioutil"
	"net"
	"os"
)

func CmdStart(c *cli.Context) error {
	config := &acme_service.AcmeProcessConfig{
		CaDir:       c.String("ca-dir"),
		RemainDays:  c.Int("cert-days"),
		Interval:    c.Duration("interval"),
		LockTimeout: c.Duration("lock-timeout"),
		InstanceId:  xid.New().String(),
	}
	sitesConfig := &common.SitesConfig{}
	f, err := os.Open(c.String("config"))
	if err != nil {
		panic(err)
	}
	configBytes, err := ioutil.ReadAll(f)
	if err != nil {
		panic(err)
	}
	f.Close()
	err = yaml.Unmarshal(configBytes, sitesConfig)
	if err != nil {
		panic(err)
	}

	store := MustInitStore(c)
	acmeService := acme_service.NewAcmeService(config, sitesConfig, store)
	acmeService.StartLoop()

	update := acmeService.NotificationChannel()
	xds := xds_service.NewXdsService()
	ctx := context.Background()
	lis, err := net.Listen("tcp", c.String("xds-listen"))
	if err != nil {
		panic(err)
	}

	stop := make(chan struct{})
	go func() {
		err = xds.RunServer(ctx, lis, update)
		if err != nil {
			panic(err)
		}
		stop <- struct{}{}
	}()

	acmeService.FireNotification()

	<-stop
	return nil
}
