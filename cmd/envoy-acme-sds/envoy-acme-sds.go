package main

import (
	"context"
	"fmt"
	"github.com/ghodss/yaml"
	"github.com/joho/godotenv"
	"github.com/kamijin-fanta/envoy-acme-sds/pkg/acme_service"
	"github.com/kamijin-fanta/envoy-acme-sds/pkg/common"
	"github.com/kamijin-fanta/envoy-acme-sds/pkg/store"
	"github.com/kamijin-fanta/envoy-acme-sds/pkg/store/consul_store"
	"github.com/kamijin-fanta/envoy-acme-sds/pkg/store/file_store"
	"github.com/kamijin-fanta/envoy-acme-sds/pkg/xds_service"
	"github.com/rs/xid"
	"github.com/urfave/cli/v2"
	"io/ioutil"
	"log"
	"net"
	"os"
	"time"
)

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	app := &cli.App{
		Name: "envoy-acme-sds",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "ca-dir",
				Value:   "https://acme-staging-v02.api.letsencrypt.org/directory", // todo
				EnvVars: []string{"CA_DIR"},
			},
			&cli.IntFlag{
				Name:    "cert-days",
				EnvVars: []string{"CERT_DAYS"},
				Value:   25,
			},
			&cli.StringFlag{
				Name:    "xds-listen",
				EnvVars: []string{"XDS_LISTEN"},
				Value:   "127.0.0.1:20000",
			},
			&cli.DurationFlag{
				Name:    "interval",
				EnvVars: []string{"INTERVAL"},
				Value:   1 * time.Hour,
			},
			&cli.DurationFlag{
				Name:    "lock-timeout",
				EnvVars: []string{"LOCK_TIMEOUT"},
				Value:   10 * time.Minute,
			},
			&cli.StringFlag{
				Name:    "config",
				Aliases: []string{"c"},
				EnvVars: []string{"CONFIG_FILE"},
				Value:   "sites.yaml",
			},
			&cli.StringFlag{
				Name:    "store",
				EnvVars: []string{"STORE"},
				Value:   "file",
			},
			&cli.StringFlag{
				Name:    "store-file-base",
				EnvVars: []string{"STORE_FILE_BASE"},
				Value:   "./data",
			},
			&cli.StringFlag{
				Name:    "store-consul-prefix",
				EnvVars: []string{"STORE_CONSUL_PREFIX"},
				Value:   "envoy-acme-sds/default",
			},
		},
		Action: func(c *cli.Context) error {
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

			var s store.Store
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

			acmeService := acme_service.NewAcmeService(config, sitesConfig, s)
			//acmeService.FetchCertificate()
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
		},
	}

	err = app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}
