package main

import (
	"context"
	"github.com/ghodss/yaml"
	"github.com/kamijin-fanta/envoy-acme/pkg/acme_service"
	"github.com/kamijin-fanta/envoy-acme/pkg/common"
	"github.com/kamijin-fanta/envoy-acme/pkg/xds_service"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/xid"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
	"io/ioutil"
	"net"
	"net/http"
	"os"
)

func CmdStart(c *cli.Context) error {
	level, err := logrus.ParseLevel(c.String("log-level"))
	if err != nil {
		panic(err)
	}
	logger := logrus.New()
	logger.SetLevel(level)
	switch c.String("log-format") {
	case "json", "JSON":
		logger.SetFormatter(&logrus.JSONFormatter{})
	}

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
		logger.WithError(err).Fatal("failed open config file")
	}
	configBytes, err := ioutil.ReadAll(f)
	if err != nil {
		logger.WithError(err).Fatal("failed read config file")
	}
	f.Close()
	err = yaml.Unmarshal(configBytes, sitesConfig)
	if err != nil {
		logger.WithError(err).Fatal("can not parse sites config")
	}
	logger.WithField("sites", len(sitesConfig.Sites)).Debug("sites config loaded")

	store := MustInitStore(c)
	acmeService := acme_service.NewAcmeService(config, sitesConfig, store, logger)
	acmeService.StartLoop()

	update := acmeService.NotificationChannel()
	xds := xds_service.NewXdsService(logger)
	ctx := context.Background()
	xdsLis, err := net.Listen("tcp", c.String("xds-listen"))
	if err != nil {
		logger.WithError(err).Fatal("failed open xds listener")
	}

	stop := make(chan struct{})
	go func() {
		err = xds.RunServer(ctx, xdsLis, update)
		if err != nil {
			logger.WithError(err).Fatal("failed run xds server")
		}
		stop <- struct{}{}
	}()
	go func() {
		http.Handle("/metrics", promhttp.Handler())
		addr := c.String("metrics-listen")
		logger.WithField("addr", addr).Info("start metrics http server")
		err := http.ListenAndServe(addr, nil)
		if err != nil {
			logger.WithError(err).Fatal("failed run metrics http server")
		}
		stop <- struct{}{}
	}()

	acmeService.FireNotification()

	<-stop
	return nil
}
