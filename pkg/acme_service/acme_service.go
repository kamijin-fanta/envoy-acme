package acme_service

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
	"github.com/kamijin-fanta/envoy-acme-sds/pkg/common"
	"github.com/kamijin-fanta/envoy-acme-sds/pkg/store"
	"log"
	"os"
	"strings"
	"time"
)

type AcmeService struct {
	Config              *AcmeProcessConfig
	SitesConfig         *common.SitesConfig
	Store               store.Store
	notificationChannel chan *common.Notification
}

func NewAcmeService(config *AcmeProcessConfig, sitesConfig *common.SitesConfig, store store.Store) *AcmeService {
	return &AcmeService{
		Config:              config,
		SitesConfig:         sitesConfig,
		Store:               store,
		notificationChannel: make(chan *common.Notification),
	}
}

type AcmeProcessConfig struct {
	CaDir      string
	RemainDays int
	Interval   time.Duration
}

func (a *AcmeService) NotificationChannel() chan *common.Notification {
	return a.notificationChannel
}

func (a *AcmeService) StartLoop() {
	go func() {
		for {
			sitesChanges := false
			for _, site := range a.SitesConfig.Sites {
				func() {
					defer func() {
						if e := recover(); e != nil {
							log.Printf("fetch certificate panic on %s %v\n", site.Name, e)
						}
					}()
					fmt.Printf("Start check certificate for %s\n", site.Name)

					// unset all relative env vars
					for _, site := range a.SitesConfig.Sites {
						for _, env := range site.LegoEnv {
							vars := strings.Split(env, "=")
							os.Unsetenv(vars[0])
						}
					}
					result := a.FetchCertificate(site)
					if result {
						sitesChanges = true
					}
				}()
			}

			if sitesChanges {
				a.FireNotification()
			}

			// wait for timer
			t := time.NewTimer(a.Config.Interval)
			<-t.C
		}
	}()
}

func (a *AcmeService) FetchCertificate(site *common.Site) bool {
	resource, err := a.Store.FetchResource(site.Name)
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

		if !needRenewal(certs[0], a.Config.RemainDays) {
			log.Println("not need renewal")
			return false
		}
		log.Println("need renewal")
	}

	account, err := a.Store.FetchUser(a.Config.CaDir, site.Email)
	if errors.Is(err, store.ErrNotFoundUser) {
		// regist new user
		log.Println("generate user private key")

		privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			log.Fatal(err) // todo fix
		}

		newAccount := store.NewAccount(site.Email, privateKey)
		clientConfig := lego.NewConfig(newAccount)

		clientConfig.CADirURL = a.Config.CaDir

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

		err = a.Store.WriteUser(a.Config.CaDir, newAccount)
		if err != nil {
			log.Fatalf("write user error %v", err)
		}
	} else if err != nil {
		log.Fatalf("error on fetch user %v", err)
	}

	clientConfig := lego.NewConfig(account)
	clientConfig.Certificate.KeyType = certcrypto.RSA2048
	clientConfig.CADirURL = a.Config.CaDir

	client, err := lego.NewClient(clientConfig)
	if err != nil {
		log.Fatal(err) // todo fix
	}

	// set EnvVars
	for _, env := range site.LegoEnv {
		vars := strings.Split(env, "=")
		if len(vars) != 2 {
			fmt.Printf("ignore invalid env vars %s\n", env)
			continue
		}
		err := os.Setenv(vars[0], vars[1])
		fmt.Printf("set env %v\n", vars[0])
		if err != nil {
			panic(err)
		}
	}
	provider, err := dns.NewDNSChallengeProviderByName(site.Provider)
	if err != nil {
		log.Fatal(err) // todo fix
	}
	err = client.Challenge.SetDNS01Provider(provider)
	if err != nil {
		log.Fatal(err) // todo fix
	}

	fmt.Printf("start acme processes...\n")

	request := certificate.ObtainRequest{
		Domains: site.Domains,
		Bundle:  true,
	}
	certificates, err := client.Certificate.Obtain(request)
	if err != nil {
		log.Fatal(err) // todo fix
	}
	certResource := store.NewStoreResource(certificates)

	fmt.Printf("%s\n", certificates.Certificate)

	err = a.Store.WriteResource(site.Name, certResource)
	if err != nil {
		log.Fatalf("issue certificate error %v", err) // todo fix
	}
	return true
}

func (a *AcmeService) FireNotification() {
	certs := make([]*store.Certificates, 0, len(a.SitesConfig.Sites))
	for _, site := range a.SitesConfig.Sites {
		cert, err := a.Store.FetchResource(site.Name)
		if err != nil {
			fmt.Printf("error on fetch resource %v\n", err)
			continue
		}
		certs = append(certs, cert)
	}
	a.notificationChannel <- &common.Notification{
		Certificates: certs,
	}
}

func needRenewal(x509Cert *x509.Certificate, remainDay int) bool {
	if remainDay < 0 {
		return true
	}
	notAfter := int(time.Until(x509Cert.NotAfter).Hours() / 24.0)
	return notAfter <= remainDay
}
