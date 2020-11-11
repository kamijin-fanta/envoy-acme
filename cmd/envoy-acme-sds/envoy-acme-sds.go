package main

import (
	"context"
	"fmt"
	"github.com/joho/godotenv"
	"github.com/kamijin-fanta/envoy-acme-sds/pkg/acme_service"
	"github.com/kamijin-fanta/envoy-acme-sds/pkg/common"
	"github.com/kamijin-fanta/envoy-acme-sds/pkg/store"
	"github.com/kamijin-fanta/envoy-acme-sds/pkg/store/file_store"
	"github.com/kamijin-fanta/envoy-acme-sds/pkg/xds_service"
	"github.com/urfave/cli/v2"
	"log"
	"net"
	"os"
	"strings"
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
			&cli.StringFlag{
				Name:     "email",
				EnvVars:  []string{"EMAIL"},
				Required: true,
			},
			&cli.StringFlag{
				Name:     "dns01-provider",
				EnvVars:  []string{"DNS01_PROVIDER"},
				Required: true,
			},
			&cli.StringFlag{
				Name:     "domains",
				EnvVars:  []string{"DOMAINS"},
				Required: true,
			},
			&cli.IntFlag{
				Name:    "cert-days",
				EnvVars: []string{"CERT_DAYS"},
				Value:   25,
			},
			&cli.StringFlag{
				Name:    "file-store-dir",
				EnvVars: []string{"FILE_STORE_DIR"},
				Value:   "./data",
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
		},
		Action: func(c *cli.Context) error {
			config := &acme_service.AcmeProcessConfig{
				CaDir:             c.String("ca-dir"),
				Email:             c.String("email"),
				Dns01ProviderName: c.String("dns01-provider"),
				Domains:           strings.Split(c.String("domains"), ","),
				RemainDays:        c.Int("cert-days"),
				DataDir:           c.String("file-store-dir"),
				Interval:          c.Duration("interval"),
			}

			os.MkdirAll(config.DataDir, 0700)
			fileStore := file_store.NewFileStore(config.DataDir)

			acmeService := acme_service.NewAcmeService(config, fileStore)
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

			r, err := fileStore.FetchResource(config.Domains[0])
			if err != nil {
				fmt.Printf("initial fetch error %v\n", err)
			}
			update <- &common.Notification{
				Certificates: []*store.Certificates{r},
			}

			<-stop
			return nil
		},
	}

	err = app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}
