package file_store

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/kamijin-fanta/envoy-acme-sds/pkg/store"
	"io/ioutil"
	"net/url"
	"os"
	"path"
	"strings"
)

type FileStore struct {
	baseFilePath string
}

func NewFileStore(base string) *FileStore {
	return &FileStore{
		baseFilePath: base,
	}
}

func (f *FileStore) FetchUser(caServer string, userId string) (*store.Account, error) {
	userPath, err := userFilePath(f.baseFilePath, caServer, userId)
	if err != nil {
		return nil, err
	}

	fi, err := os.Open(userPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil, store.ErrNotFoundUser
	}
	if err != nil {
		return nil, err
	}
	defer fi.Close()

	content, err := ioutil.ReadAll(fi)
	if err != nil {
		return nil, err
	}

	account := new(store.Account)
	err = json.Unmarshal(content, account)
	if err != nil {
		return nil, err
	}

	return account, nil
}
func (f *FileStore) WriteUser(caServer string, account *store.Account) error {
	userPath, err := userFilePath(f.baseFilePath, caServer, account.Email)
	if err != nil {
		return err
	}

	jsonBytes, err := json.Marshal(account)
	if err != nil {
		return err
	}

	err = ioutil.WriteFile(userPath, jsonBytes, 0700)
	if err != nil {
		return err
	}
	return nil
}
func (f *FileStore) FetchResource(symbolicDomainName string) (*store.Certificates, error) {
	resourcePath := resourceFilePath(f.baseFilePath, symbolicDomainName)

	fi, err := os.Open(resourcePath)
	if errors.Is(err, os.ErrNotExist) {
		return nil, store.ErrNotFoundCertificate
	}
	if err != nil {
		return nil, err
	}
	defer fi.Close()

	content, err := ioutil.ReadAll(fi)
	if err != nil {
		return nil, err
	}

	certs := new(store.Certificates)
	err = json.Unmarshal(content, certs)
	if err != nil {
		return nil, err
	}

	return certs, nil
}
func (f *FileStore) WriteResource(symbolicDomainName string, resource *store.Certificates) error {
	resourcePath := resourceFilePath(f.baseFilePath, symbolicDomainName)
	jsonBytes, err := json.Marshal(resource)
	if err != nil {
		return err
	}

	err = ioutil.WriteFile(resourcePath, jsonBytes, 0700)
	if err != nil {
		return err
	}
	return nil
}

func userFilePath(base, caServer, userId string) (string, error) {
	serverUrl, err := url.Parse(caServer)
	if err != nil {
		return "", err
	}
	serverPath := strings.NewReplacer(":", "_", "/", "_").Replace(serverUrl.Host)

	return path.Join(base, fmt.Sprintf("user-%s-%s.json", serverPath, userId)), nil
}

func resourceFilePath(base, domainName string) string {
	return path.Join(base, fmt.Sprintf("resource-%s.json", domainName))
}
