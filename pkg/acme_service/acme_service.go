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
	"github.com/sirupsen/logrus"
	"os"
	"strings"
	"time"
)

type AcmeService struct {
	Config              *AcmeProcessConfig
	SitesConfig         *common.SitesConfig
	Store               store.Store
	notificationChannel chan *common.Notification
	logger              *logrus.Entry
}

func NewAcmeService(config *AcmeProcessConfig, sitesConfig *common.SitesConfig, store store.Store, logger *logrus.Logger) *AcmeService {
	return &AcmeService{
		Config:              config,
		SitesConfig:         sitesConfig,
		Store:               store,
		notificationChannel: make(chan *common.Notification),
		logger:              logger.WithField("component", "acme_service"),
	}
}

type AcmeProcessConfig struct {
	CaDir       string
	RemainDays  int
	Interval    time.Duration
	LockTimeout time.Duration
	InstanceId  string
}

func (a *AcmeService) NotificationChannel() chan *common.Notification {
	return a.notificationChannel
}

func (a *AcmeService) StartLoop() {
	go func() {
		for {
			sitesChanges := false
			for _, site := range a.SitesConfig.Sites {
				siteLogger := a.logger.WithField("site", site.Name)
				func() {
					for retry := 0; true; retry += 1 {
						ok, err := a.Store.Lock(a.Config.InstanceId, a.Config.LockTimeout)
						if err != nil {
							siteLogger.WithField("retry", retry).WithError(err).Debug("lock error")
						}
						if ok {
							// success!
							break
						}
						if retry > 10 {
							siteLogger.WithField("retry", retry).Info("Skip because the lock cannot be obtained.")
							return
						}
						siteLogger.WithField("retry", retry).Debug("lock failed")
						wait := 5 * time.Second
						siteLogger.WithField("duration", wait.String()).Debug("wait for lock")
						time.Sleep(wait)
					}
					defer a.Store.Release(a.Config.InstanceId)
					siteLogger.WithField("instance", a.Config.InstanceId).Debug("success lock")

					defer func() {
						if e := recover(); e != nil {
							siteLogger.WithField("error", e).Warn("panic fetch certificate")
						}
					}()
					siteLogger.Debug("check certificate")

					// unset all relative env vars
					for _, site := range a.SitesConfig.Sites {
						for _, env := range site.LegoEnv {
							vars := strings.Split(env, "=")
							os.Unsetenv(vars[0])
						}
					}
					result, err := a.FetchCertificate(site)
					if err != nil {
						siteLogger.WithError(err).Warn("renewal error")
					}
					if result {
						sitesChanges = true
						siteLogger.Info("renewal success")
					} else {
						siteLogger.Info("not need renewal")
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

func (a *AcmeService) FetchCertificate(site *common.Site) (bool, error) {
	siteLogger := a.logger.WithField("site", site.Name)

	resource, err := a.Store.FetchResource(site.Name)
	if errors.Is(err, store.ErrNotFoundCertificate) {
		// nop
	} else if err != nil {
		return false, fmt.Errorf("fetch resource error %w", err)
	} else {
		// check expiration date
		certs, err := resource.ExtractCertificate()
		if err != nil {
			return false, fmt.Errorf("extract certs error %w", err)
		}
		if len(certs) != 0 {
			if !needRenewal(certs[0], a.Config.RemainDays) {
				return false, nil
			}
		}
	}

	account, err := a.Store.FetchUser(a.Config.CaDir, site.Email)
	if errors.Is(err, store.ErrNotFoundUser) {
		// regist new user
		siteLogger.WithField("email", site.Email).Info("generate user private key")

		privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			return false, fmt.Errorf("error generate private key %w", err)
		}

		newAccount := store.NewAccount(site.Email, privateKey)
		clientConfig := lego.NewConfig(newAccount)

		clientConfig.CADirURL = a.Config.CaDir

		client, err := lego.NewClient(clientConfig)
		if err != nil {
			return false, fmt.Errorf("error create new lego client %w", err)
		}

		reg, err := client.Registration.Register(registration.RegisterOptions{TermsOfServiceAgreed: true})
		if err != nil {
			return false, fmt.Errorf("error acme user registration %w", err)
		}
		newAccount.Registration = reg
		account = newAccount

		err = a.Store.WriteUser(a.Config.CaDir, newAccount)
		if err != nil {
			return false, fmt.Errorf("error write new user %w", err)
		}
	} else if err != nil {
		return false, fmt.Errorf("error on fetch user %w", err)
	}

	clientConfig := lego.NewConfig(account)
	clientConfig.Certificate.KeyType = certcrypto.RSA2048
	clientConfig.CADirURL = a.Config.CaDir

	client, err := lego.NewClient(clientConfig)
	if err != nil {
		return false, fmt.Errorf("error create new lego client %w", err)
	}

	// set EnvVars
	for _, env := range site.LegoEnv {
		vars := strings.Split(env, "=")
		if len(vars) != 2 {
			siteLogger.WithField("variable", env).Info("ignore invalid env vars")
			continue
		}
		err := os.Setenv(vars[0], vars[1])
		siteLogger.WithField("key", vars[0]).Trace("set env var")
		if err != nil {
			panic(err)
		}
	}
	provider, err := dns.NewDNSChallengeProviderByName(site.Provider)
	if err != nil {
		return false, fmt.Errorf("error on new provider %w", err)
	}
	err = client.Challenge.SetDNS01Provider(provider)
	if err != nil {
		return false, fmt.Errorf("error on set provider %w", err)
	}

	request := certificate.ObtainRequest{
		Domains: site.Domains,
		Bundle:  true,
	}
	siteLogger.WithField("request", request).Debug("start obtain request")
	certificates, err := client.Certificate.Obtain(request)
	if err != nil {
		return false, fmt.Errorf("error obtain certificate %w", err)
	}
	certResource := store.NewStoreResource(certificates)

	err = a.Store.WriteResource(site.Name, certResource)
	if err != nil {
		return false, fmt.Errorf("issue certificate error %w", err)
	}
	return true, nil
}

func (a *AcmeService) FireNotification() {
	certs := make([]*store.Certificates, 0, len(a.SitesConfig.Sites))
	for _, site := range a.SitesConfig.Sites {
		cert, err := a.Store.FetchResource(site.Name)
		if err != nil {
			a.logger.WithError(err).Warn("error on fetch resource")
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
