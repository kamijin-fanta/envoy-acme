package main

import (
	"github.com/joho/godotenv"
	"github.com/urfave/cli/v2"
	"log"
	"os"
	"time"
)

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	app := &cli.App{
		Name: "envoy-acme",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "log-level",
				EnvVars: []string{"LOG_LEVEL"},
				Value:   "info",
			},
			&cli.StringFlag{
				Name:    "log-format",
				EnvVars: []string{"LOG_FORMAT"},
				Value:   "text",
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
				Value:   "envoy-acme/default",
			},
		},
		Commands: []*cli.Command{
			{
				Name:  "start",
				Usage: "start sds server",
				Flags: []cli.Flag{

					&cli.StringFlag{
						Name:    "ca-dir",
						Value:   "https://acme-v02.api.letsencrypt.org/directory",
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
						Name:    "metrics-listen",
						EnvVars: []string{"METRICS_LISTEN"},
						Value:   "127.0.0.1:20001",
					},
				},
				Action: CmdStart,
			},
			{
				Name:  "export",
				Usage: "export cert, keys file from store",
				Flags: []cli.Flag{
					&cli.StringSliceFlag{
						Name:     "name",
						Usage:    "target configure name",
						Required: true,
					},
					&cli.StringFlag{
						Name:  "dest",
						Usage: "output directory",
						Value: ".",
					},
				},
				Action: CmdExport,
			},
		},
	}

	err = app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}
