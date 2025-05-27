package server

import (
	"context"
	"log"
	"strconv"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/codes"
	// "go.etcd.io/etcd/api/v3/version" // Keep commented for now, GetVersion needs rework
	"errors"
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
	cfg    *EtcdWrapConfig
	client *clientv3.Client
}

// NewEtcdWrap create a new etcd wrap.
func NewEtcdWrap(cfg *EtcdWrapConfig) (*EtcdWrap, error) {
	log.Printf("etcd wrap config: %+v", cfg)

	cli, err := clientv3.New(clientv3.Config{
		Endpoints:   cfg.Endpoints,
		Username:    cfg.UserName,
		Password:    cfg.Password,
		DialTimeout: 5 * time.Second, // Example timeout
	})

	if err != nil {
		log.Printf("Failed to connect to etcd: %v", err)
		return nil, err
	}

	// TODO: Consider a way to check connectivity, e.g., by getting cluster status or a dummy key.
	// For now, we assume successful connection if clientv3.New doesn't error.

	w := &EtcdWrap{
		cfg:    cfg,
		client: cli,
	}
	return w, nil
}

// GetVersion has been removed as it requires using the Maintenance API client,
// which is a larger change than the current scope of refactoring core KV operations.
// Placeholder functionality might be re-added if simple status checks are needed.

// GetNCreate retrieves a key, creating it with createValue if it does not exist.
// Returns the GetResponse and a boolean indicating if the key was created.
func (w *EtcdWrap) GetNCreate(key string, createValue int) (*clientv3.GetResponse, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second) // Increased timeout for Txn
	defer cancel()

	// Convert createValue to string
	sCreateValue := strconv.Itoa(createValue)

	// Transaction to create if not exists
	txn := w.client.KV.Txn(ctx)
	// If key does not exist (its version is 0), then create it.
	// Else, just get it.
	resp, err := txn.If(clientv3.Compare(clientv3.Version(key), "=", 0)).
		Then(clientv3.OpPut(key, sCreateValue)).
		Else(clientv3.OpGet(key)).
		Commit()

	if err != nil {
		log.Printf("GetNCreate failed for key '%s': %v", key, err)
		return nil, false, err
	}

	if !resp.Succeeded {
		// Key already existed, Else path (OpGet) was executed.
		// The GetResponse is in resp.Responses[0].GetResponseRange().
		// Important: TxnResponse.Responses is a slice of ResponseOp_Response.
		// We need to type assert to get the specific response type.
		getResponse := resp.Responses[0].GetResponseRange()
		if getResponse == nil {
			return nil, false, errors.New("etcd GetNCreate: key existed but failed to retrieve in transaction")
		}
		return (*clientv3.GetResponse)(getResponse), false, nil
	}

	// Key did not exist and was created (Then path - OpPut).
	// We need to fetch the created key to return a GetResponse consistent with prior behavior.
	// The PutResponse is in resp.Responses[0].GetResponsePut().
	// We could return this PutResponse's header, or do a Get. Let's do a Get for consistency.
	getResp, err := w.Get(key)
	if err != nil {
		log.Printf("GetNCreate: key '%s' created, but failed to retrieve after creation: %v", key, err)
		return nil, true, err
	}
	return getResp, true, nil
}


// AtomAdd atomically adds an integer value to the current integer value of a key.
// It retries if there's a conflict during the read-modify-write cycle.
func (w *EtcdWrap) AtomAdd(key string, value int) (int, error) {
	for {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second) // Context for each attempt

		// Get current value and revision
		getResp, err := w.client.KV.Get(ctx, key)
		if err != nil {
			cancel()
			log.Printf("AtomAdd: failed to get key '%s': %v", key, err)
			return 0, err
		}

		currentValueStr := "0" // Default to 0 if key doesn't exist or has no value
		currentRevision := int64(0)

		if len(getResp.Kvs) > 0 {
			currentValueStr = string(getResp.Kvs[0].Value)
			currentRevision = getResp.Kvs[0].ModRevision
		}

		currentValueInt, err := strconv.Atoi(currentValueStr)
		if err != nil {
			cancel()
			log.Printf("AtomAdd: value of key '%s' ('%s') is not an integer: %v", key, currentValueStr, err)
			return 0, errors.New("current value is not an integer")
		}

		newValueInt := currentValueInt + value
		newValueStr := strconv.Itoa(newValueInt)

		// Start transaction
		txn := w.client.KV.Txn(ctx)
		var KVCmp clientv3.Cmp
		if currentRevision == 0 {
			// Key does not exist, try to create it (version is 0)
			KVCmp = clientv3.Compare(clientv3.Version(key), "=", 0)
		} else {
			// Key exists, compare based on ModRevision
			KVCmp = clientv3.Compare(clientv3.ModRevision(key), "=", currentRevision)
		}
		
		txnResp, err := txn.If(KVCmp).
			Then(clientv3.OpPut(key, newValueStr)).
			Commit()
		
		cancel() // Release context resources for this attempt

		if err != nil {
			log.Printf("AtomAdd: transaction failed for key '%s': %v", key, err)
			return 0, err // Or retry a few times for certain errors
		}

		if txnResp.Succeeded {
			// Transaction successful, value updated
			return newValueInt, nil
		}

		// Transaction failed, likely due to a race condition (ModRevision changed).
		// Loop will retry.
		log.Printf("AtomAdd: conflict for key '%s', retrying...", key)
		// Optional: add a small delay or backoff here if conflicts are frequent
		time.Sleep(10 * time.Millisecond) 
	}
}

// Get retrieves a single key-value pair from etcd.
func (w *EtcdWrap) Get(key string) (*clientv3.GetResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second) // Example timeout
	defer cancel()

	resp, err := w.client.KV.Get(ctx, key)
	if err != nil {
		log.Printf("Failed to get key '%s' from etcd: %v", key, err)
		return nil, err
	}
	return resp, nil
}

// Set assigns a new value to a key.
func (w *EtcdWrap) Set(key string, value string) (*clientv3.PutResponse, error) {
	return w.SetWithTTL(key, value, 0) // TTL 0 means no lease
}

// SetWithTTL assigns a new value to a key with a specified Time To Live (TTL) in seconds.
// If ttl is 0, the key is persisted without a TTL.
func (w *EtcdWrap) SetWithTTL(key string, value string, ttlSeconds int64) (*clientv3.PutResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second) // Increased timeout for potential lease grant
	defer cancel()

	var opts []clientv3.OpOption
	if ttlSeconds > 0 {
		leaseResp, err := w.client.Grant(ctx, ttlSeconds)
		if err != nil {
			log.Printf("Failed to grant lease for TTL on key '%s': %v", key, err)
			return nil, err
		}
		opts = append(opts, clientv3.WithLease(leaseResp.ID))
	}

	resp, err := w.client.KV.Put(ctx, key, value, opts...)
	if err != nil {
		log.Printf("Failed to set key '%s' with TTL %d: %v", key, ttlSeconds, err)
		return nil, err
	}
	return resp, nil
}

// Delete removes a Node identified by the given key.
// For deleting a range of keys (prefix), use DeleteWithPrefix.
func (w *EtcdWrap) Delete(key string) (*clientv3.DeleteResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := w.client.KV.Delete(ctx, key)
	if err != nil {
		log.Printf("Failed to delete key '%s' from etcd: %v", key, err)
		return nil, err
	}
	return resp, nil
}

// DeleteWithPrefix removes all keys matching the given prefix.
func (w *EtcdWrap) DeleteWithPrefix(prefix string) (*clientv3.DeleteResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := w.client.KV.Delete(ctx, prefix, clientv3.WithPrefix())
	if err != nil {
		log.Printf("Failed to delete keys with prefix '%s' from etcd: %v", prefix, err)
		return nil, err
	}
	return resp, nil
}

// IsKeyExist returns true if the error indicates that a key already exists.
// This is typically used with transactions that try to create a key.
// For clientv3, if a Txn fails due to a Compare (e.g., key version is not 0),
// the TxnResponse.Succeeded will be false.
func (w *EtcdWrap) IsKeyExist(txnResp *clientv3.TxnResponse, err error) bool {
	if err != nil {
		// Some gRPC errors might indicate this, but usually it's a failed transaction.
		// e.g. status.Code(err) == codes.AlreadyExists, though this is less common for typical "create if not exist"
		return false
	}
	// If a transaction failed (e.g. "if version == 0 then put" failed),
	// it implies the condition wasn't met, meaning the key likely existed.
	return txnResp != nil && !txnResp.Succeeded
}

// CompareAndSwap atomically sets the value of a key if its current ModRevision matches expectedModRevision.
// If expectedModRevision is 0, it compares against version=0 (key does not exist).
func (w *EtcdWrap) CompareAndSwap(key string, expectedModRevision int64, newValue string) (*clientv3.TxnResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	txn := w.client.KV.Txn(ctx)
	var cmp clientv3.Cmp
	if expectedModRevision == 0 { // Expect key to not exist
		cmp = clientv3.Compare(clientv3.Version(key), "=", 0)
	} else { // Expect key to exist with specific revision
		cmp = clientv3.Compare(clientv3.ModRevision(key), "=", expectedModRevision)
	}

	txnResp, err := txn.If(cmp).
		Then(clientv3.OpPut(key, newValue)).
		Commit()

	if err != nil {
		log.Printf("CompareAndSwap: transaction failed for key '%s': %v", key, err)
		return nil, err
	}
	return txnResp, nil
}

// IsKeyNotFound checks if a GetResponse indicates a key was not found or if an error is codes.NotFound.
func (w *EtcdWrap) IsKeyNotFound(resp *clientv3.GetResponse, err error) bool {
	if err != nil {
		st, ok := status.FromError(err)
		if ok && st.Code() == codes.NotFound {
			return true
		}
		return false // Some other error occurred, not specifically "NotFound"
	}
	// If no error, a non-existent key means the GetResponse has no KVs.
	return resp != nil && resp.Count == 0
}
