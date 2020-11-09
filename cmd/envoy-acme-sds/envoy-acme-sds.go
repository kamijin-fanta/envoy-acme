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
	"log"
	"os"
	"time"
)

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	caDir := "https://acme-staging-v02.api.letsencrypt.org/directory"
	email := "test@you.com"
	dns01ProviderName := "sakuracloud"
	domains := []string{"metrics.dempa.moe", "*.metrics.dempa.moe"}
	remainDays := 10
	dataDir := "./data"

	os.MkdirAll(dataDir, 0700)
	fileStore := file_store.NewFileStore(dataDir)

	resource, err := fileStore.FetchResource(domains[0])
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

		if !needRenewal(certs[0], remainDays) {
			log.Println("not need renewal")
			return
		}
		log.Println("need renewal")
	}

	account, err := fileStore.FetchUser(caDir, email)
	if errors.Is(err, store.ErrNotFoundUser) {
		// regist new user
		log.Println("generate user private key")

		privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			log.Fatal(err)
		}

		newAccount := store.NewAccount(email, privateKey)
		config := lego.NewConfig(newAccount)

		config.CADirURL = caDir

		client, err := lego.NewClient(config)
		if err != nil {
			log.Fatal(err)
		}

		reg, err := client.Registration.Register(registration.RegisterOptions{TermsOfServiceAgreed: true})
		if err != nil {
			log.Fatal(err)
		}
		newAccount.Registration = reg
		account = newAccount

		err = fileStore.WriteUser(caDir, newAccount)
		if err != nil {
			log.Fatalf("write user error %v", err)
		}
	} else if err != nil {
		log.Fatalf("error on fetch user %v", err)
	}

	config := lego.NewConfig(account)
	config.Certificate.KeyType = certcrypto.RSA2048
	config.CADirURL = caDir

	client, err := lego.NewClient(config)
	if err != nil {
		log.Fatal(err)
	}

	provider, err := dns.NewDNSChallengeProviderByName(dns01ProviderName)
	if err != nil {
		log.Fatal(err)
	}
	err = client.Challenge.SetDNS01Provider(provider)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("start acme processes...\n")

	request := certificate.ObtainRequest{
		Domains: domains,
		Bundle:  true,
	}
	certificates, err := client.Certificate.Obtain(request)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("%#v\n", certificates)

	err = fileStore.WriteResource(domains[0], store.NewStoreResource(certificates))
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
