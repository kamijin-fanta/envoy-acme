package consul_store

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"fmt"
	"testing"
	"time"

	"github.com/go-acme/lego/v4/certificate"
	"github.com/pfremm/envoy-acme/pkg/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConsulStore(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	keyPrefix := fmt.Sprintf("envoy-acme/test-%d", time.Now().Unix())
	consulStore, err := NewConsulStore(keyPrefix)
	require.Nil(err)

	defer func() {
		// cleanup
		consulStore.kvClient.DeleteTree(keyPrefix, nil)
	}()

	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.Nil(err)

	email := "test@example.com"
	testAccount := &store.Account{
		Email:      email,
		AccountKey: store.NewAccountKey(privateKey),
	}
	LEDirectoryStaging := "https://acme-staging-v02.api.letsencrypt.org/directory"
	err = consulStore.WriteUser(LEDirectoryStaging, testAccount)
	require.Nil(err)

	account, err := consulStore.FetchUser(LEDirectoryStaging, email)
	require.Nil(err)
	require.NotNil(account)
	assert.Equal(email, account.Email)
	assert.Equal(privateKey, account.GetPrivateKey())

	domain := "example.com"
	testResource := store.NewStoreResource(&certificate.Resource{
		Domain:            domain,
		CertURL:           "cert_url",
		CertStableURL:     "cert_stable_url",
		PrivateKey:        []byte("private_key"),
		Certificate:       []byte("certificate"),
		IssuerCertificate: []byte("issuer_ertificate"),
		CSR:               []byte("csr"),
	})
	err = consulStore.WriteResource(domain, testResource)
	require.Nil(err)

	response, err := consulStore.FetchResource(domain)
	require.Nil(err)
	require.NotNil(response)
	assert.EqualValues(testResource, response.ToCertificateResource())

	lockTimeout := 100 * time.Millisecond
	res, err := consulStore.Lock("a", lockTimeout)
	require.Nil(err)
	assert.True(res)
	res, err = consulStore.Lock("b", lockTimeout)
	require.Nil(err)
	assert.False(res)
	time.Sleep(lockTimeout)
	res, err = consulStore.Lock("b", lockTimeout)
	require.Nil(err)
	assert.True(res)
	res, err = consulStore.Lock("a", lockTimeout)
	require.Nil(err)
	assert.False(res)
	err = consulStore.Release("b")
	require.Nil(err)
	res, err = consulStore.Lock("a", lockTimeout)
	require.Nil(err)
	assert.True(res)
}
