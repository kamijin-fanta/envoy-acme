package file_store

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/kamijin-fanta/envoy-acme/pkg/store"
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var _ store.Store = &FileStore{}

type FileStore struct {
	baseFilePath string
}

func NewFileStore(base string) (*FileStore, error) {
	err := os.MkdirAll(base, 0700)
	if err != nil {
		return nil, err
	}
	return &FileStore{
		baseFilePath: base,
	}, nil
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

	jsonBytes, err := json.MarshalIndent(account, "", "  ")
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
	jsonBytes, err := json.MarshalIndent(resource, "", "  ")
	if err != nil {
		return err
	}

	err = ioutil.WriteFile(resourcePath, jsonBytes, 0700)
	if err != nil {
		return err
	}
	return nil
}

func (f *FileStore) Lock(id string, timeout time.Duration) (bool, error) {
	filePath := lockFilePath(f.baseFilePath)
	lockFile, err := os.OpenFile(filePath, os.O_RDWR|os.O_CREATE, 0700)
	if err != nil {
		return false, err
	}
	defer lockFile.Close()
	stat, err := lockFile.Stat()
	if err != nil {
		return false, err
	}

	currentContent, err := ioutil.ReadAll(lockFile)
	if err != nil {
		return false, err
	}

	limit := stat.ModTime().Add(timeout)
	notEmtpy := len(currentContent) != 0
	notOwned := !bytes.Equal(currentContent, []byte(id))
	lockAlive := time.Now().Before(limit)
	//fmt.Printf("comp %v %v %v", notEmtpy, notOwned, lockAlive)
	//fmt.Printf(" / vars %v %v\n", limit, time.Now())
	if notEmtpy && notOwned && lockAlive {
		return false, nil
	}

	err = lockFile.Truncate(0)
	if err != nil {
		return false, err
	}
	_, err = lockFile.WriteAt([]byte(id), 0)
	if err != nil {
		return false, err
	}

	return true, nil
}
func (f *FileStore) Release(id string) error {
	filePath := lockFilePath(f.baseFilePath)
	lockFile, err := os.OpenFile(filePath, os.O_RDWR|os.O_CREATE, 0700)
	if err != nil {
		return err
	}
	defer lockFile.Close()

	currentContent, err := ioutil.ReadAll(lockFile)
	if err != nil {
		return err
	}

	if bytes.Equal(currentContent, []byte(id)) {
		err = lockFile.Truncate(0)
		if err != nil {
			return err
		}
	}
	return nil
}

func userFilePath(base, caServer, userId string) (string, error) {
	serverUrl, err := url.Parse(caServer)
	if err != nil {
		return "", err
	}
	serverPath := strings.NewReplacer(":", "_", "/", "_").Replace(serverUrl.Host)

	return filepath.Join(base, fmt.Sprintf("user-%s-%s.json", serverPath, userId)), nil
}

func resourceFilePath(base, domainName string) string {
	return filepath.Join(base, fmt.Sprintf("resource-%s.json", domainName))
}

func lockFilePath(base string) string {
	return filepath.Join(base, "leader")
}
