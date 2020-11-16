package consul_store

import (
	"encoding/json"
	"fmt"
	"github.com/hashicorp/consul/api"
	"github.com/kamijin-fanta/envoy-acme/pkg/store"
	"net/url"
	"path"
	"strings"
	"time"
)

var _ store.Store = &ConsulStore{}

func NewConsulStore(keyPrefix string) (*ConsulStore, error) {
	config := api.DefaultConfig()
	client, err := api.NewClient(config)
	if err != nil {
		return nil, err
	}

	return &ConsulStore{
		keyPrefix: keyPrefix,
		consul:    client,
		kvClient:  client.KV(),
	}, nil
}

type ConsulStore struct {
	keyPrefix string
	consul    *api.Client
	kvClient  *api.KV
}

func (c *ConsulStore) FetchUser(caServer string, userId string) (*store.Account, error) {
	key, err := userKey(c.keyPrefix, caServer, userId)
	if err != nil {
		return nil, err
	}
	res, _, err := c.kvClient.Get(key, nil)
	if err != nil {
		return nil, err
	}
	if res == nil {
		// 404 not found
		return nil, store.ErrNotFoundUser
	}

	account := new(store.Account)
	err = json.Unmarshal(res.Value, account)
	if err != nil {
		return nil, err
	}

	return account, nil
}

func (c *ConsulStore) WriteUser(caServer string, account *store.Account) error {
	key, err := userKey(c.keyPrefix, caServer, account.Email)
	if err != nil {
		return nil
	}

	content, err := json.MarshalIndent(account, "", "  ")
	if err != nil {
		return err
	}
	_, err = c.kvClient.Put(&api.KVPair{
		Key:   key,
		Value: content,
	}, nil)
	if err != nil {
		return err
	}
	return nil
}

func (c *ConsulStore) FetchResource(symbolicDomainName string) (*store.Certificates, error) {
	key := resourceKey(c.keyPrefix, symbolicDomainName)
	res, _, err := c.kvClient.Get(key, nil)
	if err != nil {
		return nil, err
	}
	if res == nil {
		// 404 not found
		return nil, store.ErrNotFoundCertificate
	}

	resource := new(store.Certificates)
	err = json.Unmarshal(res.Value, resource)
	if err != nil {
		return nil, err
	}

	return resource, nil
}

func (c *ConsulStore) WriteResource(symbolicDomainName string, resource *store.Certificates) error {
	key := resourceKey(c.keyPrefix, symbolicDomainName)

	content, err := json.MarshalIndent(resource, "", "  ")
	if err != nil {
		return err
	}
	_, err = c.kvClient.Put(&api.KVPair{
		Key:   key,
		Value: content,
	}, nil)
	if err != nil {
		return err
	}
	return nil
}

type lockObj struct {
	Id    string
	Limit time.Time
}

func (c *ConsulStore) Lock(id string, timeout time.Duration) (bool, error) {
	key := lockKey(c.keyPrefix)
	res, meta, err := c.kvClient.Get(key, nil)
	if err != nil {
		return false, err
	}
	lock := &lockObj{}
	if res != nil {
		err = json.Unmarshal(res.Value, &lock)
		if err != nil {
			return false, err
		}
	}

	notEmtpy := res != nil
	notOwned := lock.Id != id
	lockAlive := time.Now().Before(lock.Limit)
	//fmt.Printf("comp %v %v %v", notEmtpy, notOwned, lockAlive)
	//fmt.Printf(" / vars %v %v\n", lock.Limit, time.Now())
	if notEmtpy && notOwned && lockAlive {
		return false, nil
	}

	lock.Id = id
	lock.Limit = time.Now().Add(timeout)
	lockBytes, err := json.MarshalIndent(lock, "", "  ")
	if err != nil {
		return false, err
	}

	pair := &api.KVPair{
		Key:         key,
		ModifyIndex: meta.LastIndex,
		Value:       lockBytes,
	}
	if res == nil {
		_, err := c.kvClient.Put(pair, nil)
		if err != nil {
			return false, err
		}
		return true, nil
	} else {
		ok, _, err := c.kvClient.CAS(pair, nil)
		if err != nil {
			return false, err
		}
		return ok, nil
	}
}

func (c *ConsulStore) Release(id string) error {
	key := lockKey(c.keyPrefix)

	res, meta, err := c.kvClient.Get(key, nil)
	if err != nil {
		return err
	}
	lock := &lockObj{}
	if res != nil {
		err = json.Unmarshal(res.Value, &lock)
		if err != nil {
			return err
		}
	}

	_, _, err = c.kvClient.DeleteCAS(&api.KVPair{
		Key:         key,
		ModifyIndex: meta.LastIndex,
	}, nil)
	if err != nil {
		return err
	}

	return nil
}

func userKey(base, caServer, userId string) (string, error) {
	serverUrl, err := url.Parse(caServer)
	if err != nil {
		return "", err
	}
	serverPath := strings.NewReplacer(":", "_", "/", "_").Replace(serverUrl.Host)

	return path.Join(base, "user", fmt.Sprintf("%s-%s.json", serverPath, userId)), nil
}

func resourceKey(base, domainName string) string {
	return path.Join(base, "resource", fmt.Sprintf("%s.json", domainName))
}

func lockKey(base string) string {
	return path.Join(base, "leader")
}
