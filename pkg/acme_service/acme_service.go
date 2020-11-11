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
	"github.com/kamijin-fanta/envoy-acme-sds/pkg/store/file_store"
	"log"
	"time"
)

type AcmeService struct {
	Config              *AcmeProcessConfig
	FileStore           *file_store.FileStore
	notificationChannel chan *common.Notification
}

func NewAcmeService(config *AcmeProcessConfig, fileStore *file_store.FileStore) *AcmeService {
	return &AcmeService{
		Config:              config,
		FileStore:           fileStore,
		notificationChannel: make(chan *common.Notification),
	}
}

type AcmeProcessConfig struct {
	CaDir             string
	Email             string
	Dns01ProviderName string
	Domains           []string
	RemainDays        int
	DataDir           string
	Interval          time.Duration
}

func (a *AcmeService) NotificationChannel() chan *common.Notification {
	return a.notificationChannel
}

func (a *AcmeService) StartLoop() {
	go func() {
		for {
			func() {
				defer func() {
					if e := recover(); e != nil {
						log.Printf("fetch certificate panic %v\n", e)
					}
				}()
				fmt.Printf("Start check certificate\n")
				a.FetchCertificate()
			}()
			// wait for timer
			t := time.NewTimer(a.Config.Interval)
			<-t.C
		}
	}()
}

func (a *AcmeService) FetchCertificate() {
	resource, err := a.FileStore.FetchResource(a.Config.Domains[0])
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
			return
		}
		log.Println("need renewal")
	}

	account, err := a.FileStore.FetchUser(a.Config.CaDir, a.Config.Email)
	if errors.Is(err, store.ErrNotFoundUser) {
		// regist new user
		log.Println("generate user private key")

		privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			log.Fatal(err)
		}

		newAccount := store.NewAccount(a.Config.Email, privateKey)
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

		err = a.FileStore.WriteUser(a.Config.CaDir, newAccount)
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
		log.Fatal(err)
	}

	provider, err := dns.NewDNSChallengeProviderByName(a.Config.Dns01ProviderName)
	if err != nil {
		log.Fatal(err)
	}
	err = client.Challenge.SetDNS01Provider(provider)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("start acme processes...\n")

	request := certificate.ObtainRequest{
		Domains: a.Config.Domains,
		Bundle:  true,
	}
	certificates, err := client.Certificate.Obtain(request)
	if err != nil {
		log.Fatal(err)
	}
	certResource := store.NewStoreResource(certificates)

	a.notificationChannel <- &common.Notification{
		Certificates: []*store.Certificates{certResource}, // todo
	}

	fmt.Printf("%#v\n", certificates)

	err = a.FileStore.WriteResource(a.Config.Domains[0], certResource)
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
