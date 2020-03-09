package server

import (
	"log"
	"strconv"

	"github.com/coreos/etcd/client"
	"github.com/coreos/etcd/version"
	"golang.org/x/net/context"
)

// EtcdWrapConfig config struct
type EtcdWrapConfig struct {
	// Endpoints defines a set of URLs
	Endpoints []string
	// Username specifies the user credential to add as an authorization header
	UserName string
	// Password is the password for the specified user to add as an authorization header
	// to the request.
	Password string
}

// EtcdWrap Encapsulation of etcd
type EtcdWrap struct {
	cfg        *EtcdWrapConfig
	etcdClient client.Client
	etcdAPI    client.KeysAPI
}

// NewEtcdWrap create a new etcd wrap.
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

// GetVersion retrieves the current etcd server and cluster version.
func (w *EtcdWrap) GetVersion() (*version.Versions, error) {
	return w.etcdClient.GetVersion(context.Background())
}

// GetNCreate retrieves a set of Nodes from etcd, created if not present.
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

// AtomAdd add value to the value atom of key.
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

// Get retrieves a set of Nodes from etcd
func (w *EtcdWrap) Get(key string) (*client.Response, error) {
	r, err := w.etcdAPI.Get(context.Background(), key, nil)
	if err != nil {
		return nil, err
	}
	return r, nil
}

// Set assigns a new value to a Node identified by a given key.
func (w *EtcdWrap) Set(key string, value string, opts *client.SetOptions) (*client.Response, error) {
	r, err := w.etcdAPI.Set(context.Background(), key, value, opts)
	return r, err
}

// Delete removes a Node identified by the given key.
func (w *EtcdWrap) Delete(key string) (*client.Response, error) {
	reps, err := w.etcdAPI.Delete(context.Background(), key, nil)
	if err != nil {
		return nil, err
	}
	return reps, nil
}

// IsKeyExist returns true if the error code is ErrorCodeNodeExist.
func (w *EtcdWrap) IsKeyExist(err error) bool {
	if cErr, ok := err.(client.Error); ok {
		return cErr.Code == client.ErrorCodeNodeExist
	}
	return false
}

// IsKeyNotFound returns true if the error code is ErrorCodeKeyNotFound.
func (w *EtcdWrap) IsKeyNotFound(err error) bool {
	return client.IsKeyNotFound(err)
}
