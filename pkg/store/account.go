package store

import (
	"bytes"
	"crypto"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"

	"github.com/go-acme/lego/v4/certcrypto"
	"github.com/go-acme/lego/v4/registration"
)

var _ registration.User = &Account{}

type Account struct {
	Email        string                 `json:"email,omitempty"`
	Registration *registration.Resource `json:"registration,omitempty"`
	AccountKey   AccountKey             `json:"key,omitempty"`
}

func NewAccount(email string, privateKey crypto.PrivateKey) *Account {
	return &Account{
		Email:        email,
		Registration: nil,
		AccountKey:   NewAccountKey(privateKey),
	}
}

func (u *Account) GetEmail() string {
	return u.Email
}
func (u Account) GetRegistration() *registration.Resource {
	return u.Registration
}
func (u *Account) GetPrivateKey() crypto.PrivateKey {
	return u.AccountKey.Key
}

var _ json.Marshaler = &AccountKey{}
var _ json.Unmarshaler = &AccountKey{}
var ErrUnknownPrivateKeyType = errors.New("unknown private key type")

type AccountKey struct {
	Key crypto.PrivateKey
}

func (a *AccountKey) UnmarshalJSON(in []byte) error {
	var certStr string
	decerr := json.Unmarshal(in, &certStr)
	if decerr != nil {
		return decerr
	}

	keyBlock, _ := pem.Decode([]byte(certStr))

	var key interface{}
	var err error
	switch keyBlock.Type {
	case "RSA PRIVATE KEY":
		key, err = x509.ParsePKCS1PrivateKey(keyBlock.Bytes)
	case "EC PRIVATE KEY":
		key, err = x509.ParseECPrivateKey(keyBlock.Bytes)
	default:
		return ErrUnknownPrivateKeyType
	}
	if err != nil {
		return err
	}
	a.Key = key
	return nil
}

func (a *AccountKey) MarshalJSON() ([]byte, error) {
	certOut := &bytes.Buffer{}
	pemKey := certcrypto.PEMBlock(a.Key)
	err := pem.Encode(certOut, pemKey)
	if err != nil {
		return nil, err
	}
	return json.Marshal(string(certOut.Bytes()))
}

func NewAccountKey(key crypto.PrivateKey) AccountKey {
	return AccountKey{
		Key: key,
	}
}
