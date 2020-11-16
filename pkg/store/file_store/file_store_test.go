package file_store

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"github.com/go-acme/lego/v4/certificate"
	"github.com/kamijin-fanta/envoy-acme/pkg/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"io/ioutil"
	"os"
	"testing"
	"time"
)

func TestFileStore(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	tmpDir, err := ioutil.TempDir("", "acme-file-store")
	require.Nil(err)
	defer os.RemoveAll(tmpDir)

	fileStore, err := NewFileStore(tmpDir)
	require.Nil(err)

	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.Nil(err)

	email := "test@example.com"
	testAccount := &store.Account{
		Email:      email,
		AccountKey: store.NewAccountKey(privateKey),
	}
	LEDirectoryStaging := "https://acme-staging-v02.api.letsencrypt.org/directory"
	err = fileStore.WriteUser(LEDirectoryStaging, testAccount)
	require.Nil(err)

	account, err := fileStore.FetchUser(LEDirectoryStaging, email)
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
	err = fileStore.WriteResource(domain, testResource)
	require.Nil(err)

	response, err := fileStore.FetchResource(domain)
	require.Nil(err)
	require.NotNil(response)
	assert.EqualValues(testResource, response.ToCertificateResource())

	lockTimeout := 100 * time.Millisecond
	res, err := fileStore.Lock("a", lockTimeout)
	require.Nil(err)
	assert.True(res)
	res, err = fileStore.Lock("b", lockTimeout)
	require.Nil(err)
	assert.False(res)
	time.Sleep(lockTimeout)
	res, err = fileStore.Lock("b", lockTimeout)
	require.Nil(err)
	assert.True(res)
	res, err = fileStore.Lock("a", lockTimeout)
	require.Nil(err)
	assert.False(res)
	err = fileStore.Release("b")
	require.Nil(err)
	res, err = fileStore.Lock("a", lockTimeout)
	require.Nil(err)
	assert.True(res)
}
