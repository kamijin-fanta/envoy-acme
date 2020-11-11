package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"errors"
	"fmt"
	"github.com/go-acme/lego/v4/certcrypto"
	"github.com/go-acme/lego/v4/certificate"
	"github.com/go-acme/lego/v4/lego"
	"github.com/go-acme/lego/v4/providers/dns"
	"github.com/go-acme/lego/v4/registration"
	"github.com/joho/godotenv"
	"github.com/kamijin-fanta/envoy-acme-sds/pkg/store"
	"github.com/kamijin-fanta/envoy-acme-sds/pkg/store/file_store"
	"github.com/urfave/cli/v2"
	"log"
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
				Name:  "ca-dir",
				Value: "https://acme-staging-v02.api.letsencrypt.org/directory", // todo
				EnvVars: []string{"CA_DIR"},
			},
			&cli.StringFlag{
				Name:  "email",
				EnvVars: []string{"EMAIL"},
				Required: true,
			},
			&cli.StringFlag{
				Name:  "dns01-provider",
				EnvVars: []string{"DNS01_PROVIDER"},
				Required: true,
			},
			&cli.StringFlag{
				Name:  "domains",
				EnvVars: []string{"DOMAINS"},
				Required: true,
			},
			&cli.IntFlag{
				Name:  "cert-days",
				EnvVars: []string{"CERT_DAYS"},
				Value: 25,
			},
			&cli.StringFlag{
				Name:  "file-store-dir",
				EnvVars: []string{"FILE_STORE_DIR"},
				Value: "./data",
			},
		},
		Action: func(context *cli.Context) error {
			config := &acmeProcessConfig{
				caDir:             context.String("ca-dir"),
				email:             context.String("email"),
				dns01ProviderName: context.String("dns01-provider"),
				domains:           strings.Split(context.String("domains"), ","),
				remainDays:        context.Int("cert-days"),
				dataDir:           context.String("file-store-dir"),
			}

			os.MkdirAll(config.dataDir, 0700)
			fileStore := file_store.NewFileStore(config.dataDir)

			acmeProcess(config, fileStore)


			return nil
		},
	}

	err = app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}

type acmeProcessConfig struct {
	caDir             string
	email             string
	dns01ProviderName string
	domains           []string
	remainDays        int
	dataDir           string
}

func acmeProcess(config *acmeProcessConfig, fileStore *file_store.FileStore) {
	resource, err := fileStore.FetchResource(config.domains[0])
	if errors.Is(err, store.ErrNotFoundCertificate) {
		// nop
	} else if err != nil {
		log.Fatalf("fetch resource error %v", err)
	} else {
		// check expiration date
		certs, err := resource.ExtractCertificate()
		if err != nil {
			log.Fatalf("extract certs error %v", err)
		}
		if len(certs) == 0 {
			log.Fatalf("certs not found")
		}

		if !needRenewal(certs[0], config.remainDays) {
			log.Println("not need renewal")
			return
		}
		log.Println("need renewal")
	}

	account, err := fileStore.FetchUser(config.caDir, config.email)
	if errors.Is(err, store.ErrNotFoundUser) {
		// regist new user
		log.Println("generate user private key")

		privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			log.Fatal(err)
		}

		newAccount := store.NewAccount(config.email, privateKey)
		clientConfig := lego.NewConfig(newAccount)

		clientConfig.CADirURL = config.caDir

		client, err := lego.NewClient(clientConfig)
		if err != nil {
			log.Fatal(err)
		}

		reg, err := client.Registration.Register(registration.RegisterOptions{TermsOfServiceAgreed: true})
		if err != nil {
			log.Fatal(err)
		}
		newAccount.Registration = reg
		account = newAccount

		err = fileStore.WriteUser(config.caDir, newAccount)
		if err != nil {
			log.Fatalf("write user error %v", err)
		}
	} else if err != nil {
		log.Fatalf("error on fetch user %v", err)
	}

	clientConfig := lego.NewConfig(account)
	clientConfig.Certificate.KeyType = certcrypto.RSA2048
	clientConfig.CADirURL = config.caDir

	client, err := lego.NewClient(clientConfig)
	if err != nil {
		log.Fatal(err)
	}

	provider, err := dns.NewDNSChallengeProviderByName(config.dns01ProviderName)
	if err != nil {
		log.Fatal(err)
	}
	err = client.Challenge.SetDNS01Provider(provider)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("start acme processes...\n")

	request := certificate.ObtainRequest{
		Domains: config.domains,
		Bundle:  true,
	}
	certificates, err := client.Certificate.Obtain(request)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("%#v\n", certificates)

	err = fileStore.WriteResource(config.domains[0], store.NewStoreResource(certificates))
	if err != nil {
		log.Fatalf("issue certificate error %v", err)
	}
}

func needRenewal(x509Cert *x509.Certificate, remainDay int) bool {
	if remainDay < 0 {
		return true
	}
	notAfter := int(time.Until(x509Cert.NotAfter).Hours() / 24.0)
	return notAfter <= remainDay
}
