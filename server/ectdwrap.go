package server

import (
	"log"
	"strconv"

	"github.com/coreos/etcd/client"
	"github.com/coreos/etcd/version"
	"golang.org/x/net/context"
)

type EtcdWrapConfig struct {
	Endpoints []string
	UserName  string
	Password  string
}

type EtcdWrap struct {
	cfg        *EtcdWrapConfig
	etcdClient client.Client
	etcdAPI    client.KeysAPI
}

func NewEtcdWrap(cfg *EtcdWrapConfig) (w *EtcdWrap, err error) {
	w = &EtcdWrap{cfg: cfg}
	log.Printf("etcd wrap config: %v", cfg)
	etcdCfg := client.Config{
		Endpoints: cfg.Endpoints,
		Username:  cfg.UserName,
		Password:  cfg.Password,
	}
	w.etcdClient, err = client.New(etcdCfg)
	if err != nil {
		return nil, err
	}

	w.etcdAPI = client.NewKeysAPI(w.etcdClient)
	return w, nil
}

func (w *EtcdWrap) GetVersion() (*version.Versions, error) {
	return w.etcdClient.GetVersion(context.Background())
}

func (w *EtcdWrap) GetNCreate(key string, createValue int) (*client.Response, error) {
	for {
		r, err := w.etcdAPI.Get(context.Background(), key, nil)
		if err != nil {
			if client.IsKeyNotFound(err) {
				r, err := w.etcdAPI.Set(context.Background(), key, strconv.Itoa(createValue), &client.SetOptions{PrevExist: "false"})
				if err != nil {
					// recreate
					continue
				}
				// create success
				return r, nil
			}
			return nil, err
		}
		// get success
		return r, nil
	}
}

func (w *EtcdWrap) AtomAdd(key string, value int) (int, error) {
	for {
		r, err := w.etcdAPI.Get(context.Background(), key, nil)
		if err != nil {
			return 0, err
		}
		v1, err := strconv.Atoi(r.Node.Value)
		v2 := strconv.Itoa(v1 + value)
		resp, err := w.etcdAPI.Set(context.Background(), key, v2, &client.SetOptions{PrevIndex: r.Node.ModifiedIndex})
		if err != nil {
			// modify conflict, again
			continue
		}
		return strconv.Atoi(resp.Node.Value)
	}
}

func (w *EtcdWrap) Get(key string) (*client.Response, error) {
	r, err := w.etcdAPI.Get(context.Background(), key, nil)
	if err != nil {
		return nil, err
	}
	return r, nil
}

func (w *EtcdWrap) Set(key string, value string, opts *client.SetOptions) (*client.Response, error) {
	r, err := w.etcdAPI.Set(context.Background(), key, value, opts)
	return r, err
}

func (w *EtcdWrap) Delete(key string) (*client.Response, error) {
	reps, err := w.etcdAPI.Delete(context.Background(), key, nil)
	if err != nil {
		return nil, err
	}
	return reps, nil
}

func (w *EtcdWrap) IsKeyExist(err error) bool {
	if cErr, ok := err.(client.Error); ok {
		return cErr.Code == client.ErrorCodeNodeExist
	}
	return false
}

func (w *EtcdWrap) IsKeyNotFound(err error) bool {
	return client.IsKeyNotFound(err)
}
