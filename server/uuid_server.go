package server

import (
	"fmt"
	"log"
	"math/rand"
	"net"
	"strconv"
	"time"

	"github.com/cnwinds/flake/api"

	"errors"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	clientv3 "go.etcd.io/etcd/client/v3" 
)

const (
	// StartOfContainerID the first ID that the container starts to assgin.
	StartOfContainerID = 10
	// StartOfServerID the first ID that the server starts to assgin.
	StartOfServerID = 10
	// StartOfSequence the first ID that the sequence starts to assgin.
	StartOfSequence = 1
	// MaxOfSequence the maximum of the sequence.
	MaxOfSequence = 1 << 31
	// the following line used for the test of "TestOverRang"
	// MaxOfSequence = 1 << 10

	// KeyOfMaxContainerID holds the key for the maximum container ID.
	KeyOfMaxContainerID = "max_containerid"
	// KeyOfMaxServiceID holds the key for the maximum service ID.
	KeyOfMaxServiceID = "max_serviceid"

	// KeyOfContainerDir the directory where the key value is saved.
	KeyOfContainerDir = "container"
	// KeyOfServiceDir the directory where the key value is saved.
	KeyOfServiceDir = "service"
)

// Config the config used to create the server.
type Config struct {
	// Endpoints defines a set of URLs
	Endpoints []string
	// Username specifies the user credential to add as an authorization header
	UserName string
	// Password is the password for the specified user to add as an authorization header
	// to the request.
	Password string

	// ListenAddress the server listens for local address.
	ListenAddress string
	// Prefix path prefix saved in the etcd.
	Prefix string
}

// UUIDServer UUID server.
type UUIDServer struct {
	api.UnimplementedUUIDServer // For forward compatibility
	cfg        *Config
	etcdWrap   *EtcdWrap
	listen     net.Listener
	grpcServer *grpc.Server
}

// Fetch get UUID range through the server.
func (s *UUIDServer) Fetch(ctx context.Context, in *api.FetchRequest) (*api.FetchReply, error) {
	result := &api.FetchReply{}
	leftCount := int(in.NeedCount)

	// t1 := time.Now()
	// log.Printf("Fetch request: %v", in)
	// defer log.Printf("Fetch response: %v, cost time: %v", result, time.Since(t1))

	for {
		serviceID, containerID, startID, endID, err := s.getUUIDSegment(in.ServiceName, in.ContainerName, int(leftCount))
		if err != nil {
			return nil, err
		}

		item := &api.UUIDRange{ContainerId: int32(containerID), ServiceId: int32(serviceID),
			SequenceIdStart: int32(startID), SequenceIdEnd: int32(endID)}
		result.Items = append(result.Items, item)

		leftCount = leftCount - (endID - startID + 1)
		if leftCount == 0 {
			return result, nil
		}
	}
}

func (s *UUIDServer) getServieID(serviceName string) (id int, err error) {
	key := s.cfg.Prefix + "/" + KeyOfServiceDir + "/" + serviceName
	
	// Try to get the existing service ID
	resp, err := s.etcdWrap.Get(key)
	if err != nil && !s.etcdWrap.IsKeyNotFound(resp, err) { // Pass resp to IsKeyNotFound
		log.Printf("getServieID: failed to get key '%s': %v", key, err)
		return 0, err
	}

	if err == nil && resp != nil && len(resp.Kvs) > 0 {
		// Key exists
		return strconv.Atoi(string(resp.Kvs[0].Value))
	}

	// Key does not exist, try to create it by reserving a new service ID
	for {
		newServiceID, err := s.nextServiceID()
		if err != nil {
			return 0, err
		}

		// Attempt to create the key with the newServiceID if it still doesn't exist
		// GetNCreate returns the GetResponse and a boolean 'created'
		// If 'created' is true, newServiceID was successfully written.
		// If 'created' is false, another instance created it; the GetResponse contains the actual value.
		getResp, created, err := s.etcdWrap.GetNCreate(key, newServiceID)
		if err != nil {
			// Potentially retry or handle error
			log.Printf("getServieID: GetNCreate for key '%s' failed: %v. Retrying...", key, err)
			time.Sleep(100 * time.Millisecond) // Avoid tight loop on persistent errors
			continue
		}

		if created {
			return newServiceID, nil
		}
		
		// Not created by this instance, means another instance created it.
		// The getResp from GetNCreate contains the value set by the other instance (or an even earlier one).
		if getResp != nil && len(getResp.Kvs) > 0 {
			id, err = strconv.Atoi(string(getResp.Kvs[0].Value))
			if err != nil {
				log.Printf("getServieID: failed to parse existing ID for key '%s': %v", key, err)
				return 0, err
			}
			return id, nil
		}
		// This case should ideally not be reached if GetNCreate is implemented correctly
		log.Printf("getServieID: GetNCreate for key '%s' reported not created but returned no value. Retrying...", key)
		time.Sleep(100 * time.Millisecond)
	}
}

func (s *UUIDServer) nextServiceID() (id int, err error) {
	key := s.cfg.Prefix + "/" + KeyOfMaxServiceID
	result, err := s.etcdWrap.AtomAdd(key, 1)
	if err != nil {
		return 0, err
	}
	return result, nil
}

func (s *UUIDServer) getContainerID(containerName string) (id int, err error) {
	key := s.cfg.Prefix + "/" + KeyOfContainerDir + "/" + containerName

	// Try to get the existing container ID
	resp, err := s.etcdWrap.Get(key)
	if err != nil && !s.etcdWrap.IsKeyNotFound(resp, err) { // Pass resp to IsKeyNotFound
		log.Printf("getContainerID: failed to get key '%s': %v", key, err)
		return 0, err
	}

	if err == nil && resp != nil && len(resp.Kvs) > 0 {
		// Key exists
		return strconv.Atoi(string(resp.Kvs[0].Value))
	}

	// Key does not exist, try to create it
	for {
		newContainerID, err := s.nextContainerID()
		if err != nil {
			return 0, err
		}
		
		getResp, created, err := s.etcdWrap.GetNCreate(key, newContainerID)
		if err != nil {
			log.Printf("getContainerID: GetNCreate for key '%s' failed: %v. Retrying...", key, err)
			time.Sleep(100 * time.Millisecond)
			continue
		}

		if created {
			return newContainerID, nil
		}

		if getResp != nil && len(getResp.Kvs) > 0 {
			id, err = strconv.Atoi(string(getResp.Kvs[0].Value))
			if err != nil {
				log.Printf("getContainerID: failed to parse existing ID for key '%s': %v", key, err)
				return 0, err
			}
			return id, nil
		}
		log.Printf("getContainerID: GetNCreate for key '%s' reported not created but returned no value. Retrying...", key)
		time.Sleep(100 * time.Millisecond)
	}
}

func (s *UUIDServer) nextContainerID() (id int, err error) {
	key := s.cfg.Prefix + "/" + KeyOfMaxContainerID
	result, err := s.etcdWrap.AtomAdd(key, 1)
	if err != nil {
		return 0, err
	}
	return result, nil
}

// ReassignContainerID reassign an ID to the container.
func (s *UUIDServer) ReassignContainerID(containerName string) error {
	key := s.cfg.Prefix + "/" + KeyOfContainerDir + "/" + containerName
	containerID, errOuter := s.nextContainerID() 
	if errOuter != nil {
		return errOuter
	}

	for { // Retry loop for CAS
		var casResp *clientv3.TxnResponse
		var casErr error 

		getResp, getErr := s.etcdWrap.Get(key) 
		if getErr != nil {
			if s.etcdWrap.IsKeyNotFound(getResp, getErr) {
				// Key not found, try to create it. This is a "reassign" so it implies it should exist.
				// However, if it was deleted, we might want to create it.
				// For now, let's stick to the original intent of reassigning an *existing* container's ID
				// or creating if it's truly a new container name not seen before.
				// The Set below will create if not exist.
				log.Printf("ReassignContainerID: key '%s' not found during Get, attempting to create.", key)
				_, setErr := s.etcdWrap.Set(key, strconv.Itoa(containerID)) 
				if setErr != nil {
					log.Printf("ReassignContainerID: failed to set new container ID for key '%s' after not found: %v", key, setErr)
					return setErr 
				}
				return nil 
			}
			log.Printf("ReassignContainerID: failed to get key '%s' for CAS: %v", key, getErr)
			return getErr 
		}

		if len(getResp.Kvs) == 0 {
			// Key does not exist (empty GetResponse), treat as not found.
			log.Printf("ReassignContainerID: key '%s' has no value, attempting to create.", key)
			_, setErr := s.etcdWrap.Set(key, strconv.Itoa(containerID))
			if setErr != nil {
				log.Printf("ReassignContainerID: failed to set new container ID for non-existent key '%s': %v", key, setErr)
				return setErr
			}
			return nil
		}
		
		currentModRevision := getResp.Kvs[0].ModRevision
		
		casResp, casErr = s.etcdWrap.CompareAndSwap(key, currentModRevision, strconv.Itoa(containerID))
		if casErr != nil {
			log.Printf("ReassignContainerID: CompareAndSwap failed for key '%s': %v", key, casErr)
			return casErr 
		}

		if !casResp.Succeeded {
			log.Printf("ReassignContainerID: conflict for key '%s', ModRevision changed. Retrying...", key)
			time.Sleep(50 * time.Millisecond)
			continue 
		}
		return nil 
	}
}

func (s *UUIDServer) getUUIDSegment(serviceName string, containerName string, needCount int) (serviceID int, containerID int, startID int, endID int, err error) {
	// if unuse serviceName then serviceID = 1
	serviceID = 1
	containerID = 1
	if len(serviceName) > 0 {
		serviceID, err = s.getServieID(serviceName)
		if err != nil {
			return 0, 0, 0, 0, err
		}
	}

	containerID, err = s.getContainerID(containerName)
	if err != nil {
		return 0, 0, 0, 0, err
	}

	key := fmt.Sprintf("%s/%d:%d", s.cfg.Prefix, serviceID, containerID)
	for {
		// This part of the original logic for getUUIDSegment needs a major overhaul.
		// The old approach of Get -> Set is prone to race conditions.
		// A better approach is to use AtomAdd to reserve a block of IDs.
		// Let's assume the key stores the *next available start ID*.

		// Attempt to atomically add 'needCount' to the current sequence start ID
		// The AtomAdd function in etcdWrap handles the read-modify-write loop.
		// It needs to be initialized first if it doesn't exist.

		// Initialize the sequence key if it does not exist
		initResp, created, err := s.etcdWrap.GetNCreate(key, StartOfSequence)
		if err != nil {
			log.Printf("getUUIDSegment: failed to initialize sequence key '%s': %v", key, err)
			return 0,0,0,0, err
		}
		
		var currentSeqStartID int
		if created {
			currentSeqStartID = StartOfSequence
			log.Printf("getUUIDSegment: initialized sequence key '%s' to %d", key, currentSeqStartID)
		} else {
			if initResp == nil || len(initResp.Kvs) == 0 {
				log.Printf("getUUIDSegment: GetNCreate for key '%s' didn't create but returned no value", key)
				return 0,0,0,0, errors.New("failed to get or create sequence start ID")
			}
			currentSeqStartID, err = strconv.Atoi(string(initResp.Kvs[0].Value))
			if err != nil {
				log.Printf("getUUIDSegment: failed to parse current sequence start ID from '%s': %v", string(initResp.Kvs[0].Value), err)
				return 0,0,0,0, err
			}
		}
		
		// Now, atomically increment the sequence start ID.
		// This is a bit tricky because AtomAdd adds a value, but we want to get the *previous* value
		// and then add needCount to it for the *next* transaction.
		// A simpler model: AtomAdd returns the NEW value.
		// So, if AtomAdd(key, needCount) returns X, the range is [X-needCount, X-1]
		// Let's adjust AtomAdd or use a different strategy.

		// Simpler strategy for now: Read current, try to update with CAS.
		// This loop is for CAS on the sequence allocation.
		var casResp *clientv3.TxnResponse 
		var casErr error                  
		
		for { // Inner CAS loop for sequence update
			getResp, getErr := s.etcdWrap.Get(key) 
			if getErr != nil {
				log.Printf("getUUIDSegment: failed to get sequence key '%s' in CAS loop: %v", key, getErr)
				return 0,0,0,0, getErr
			}
			// Pass getErr to IsKeyNotFound
			if s.etcdWrap.IsKeyNotFound(getResp, getErr) || (getResp != nil && len(getResp.Kvs) == 0) { 
				log.Printf("getUUIDSegment: sequence key '%s' disappeared or no KVs, re-initializing.", key)
				_, _, initErr := s.etcdWrap.GetNCreate(key, StartOfSequence) 
				if initErr != nil {
					log.Printf("getUUIDSegment: failed to re-initialize key '%s': %v", key, initErr)
					return 0,0,0,0, initErr
				}
				continue // Retry the CAS Get
			}

			parseErr := error(nil) // Declare parseErr for this scope
			startID, parseErr = strconv.Atoi(string(getResp.Kvs[0].Value)) 
			if parseErr != nil {
				log.Printf("getUUIDSegment: failed to parse startID from '%s': %v", string(getResp.Kvs[0].Value), parseErr)
				return 0,0,0,0, parseErr
			}
			
			currentModRevision := getResp.Kvs[0].ModRevision

			if startID >= MaxOfSequence {
				log.Printf("getUUIDSegment: sequence for %s:%d hit MaxOfSequence (%d). Reassigning container ID.", serviceName, containerID, MaxOfSequence)
				reassignErr := s.ReassignContainerID(containerName) 
				if reassignErr != nil {
					return 0, 0, 0, 0, reassignErr
				}
				log.Printf("getUUIDSegment: Container ID for '%s' reassigned. Retrying segment acquisition.", containerName)
				return s.getUUIDSegment(serviceName, containerName, needCount) 
			}

			endID = startID + needCount - 1 
			nextStartIDForEtcd := startID + needCount

			if endID >= MaxOfSequence { 
				log.Printf("getUUIDSegment: requested count for %s:%d (%d) exceeds MaxOfSequence from startID %d. Adjusting.", serviceName, containerID, needCount, startID)
				endID = MaxOfSequence - 1 
				nextStartIDForEtcd = MaxOfSequence 
				if startID > endID { 
					log.Printf("getUUIDSegment: No IDs left for %s:%d before MaxOfSequence. Reassigning.", serviceName, containerID)
					reassignErr := s.ReassignContainerID(containerName) 
					if reassignErr != nil { return 0, 0, 0, 0, reassignErr }
					return s.getUUIDSegment(serviceName, containerName, needCount) 
				}
			}
			
			casResp, casErr = s.etcdWrap.CompareAndSwap(key, currentModRevision, strconv.Itoa(nextStartIDForEtcd))
			if casErr != nil {
				log.Printf("getUUIDSegment: CompareAndSwap for key '%s' failed: %v", key, casErr)
				return 0, 0, 0, 0, casErr
			}

			if casResp.Succeeded {
				return serviceID, containerID, startID, endID, nil
			}
			
			log.Printf("getUUIDSegment: CAS conflict for key '%s'. Retrying...", key)
			time.Sleep(50 * time.Millisecond)
		} // End of CAS retry loop
	} // End of main for loop (this was the original outer loop, now only for init)
}


func (s *UUIDServer) initUUIDData() (success bool, err error) {
	serviceResp, _, err := s.etcdWrap.GetNCreate(s.cfg.Prefix+"/"+KeyOfMaxServiceID, StartOfServerID)
	if err != nil {
		return false, err
	}
	containerResp, _, err := s.etcdWrap.GetNCreate(s.cfg.Prefix+"/"+KeyOfMaxContainerID, StartOfContainerID)
	if err != nil {
		return false, err
	}

	var serviceIDVal, containerIDVal string
	if serviceResp != nil && len(serviceResp.Kvs) > 0 {
		serviceIDVal = string(serviceResp.Kvs[0].Value)
	} else {
		// This case implies GetNCreate failed to return the value even if it existed or was just created.
		// Fallback or error, GetNCreate should ensure value is present in GetResponse if err is nil.
		log.Printf("initUUIDData: MaxServiceID key ('%s') not found or value empty after GetNCreate.", s.cfg.Prefix+"/"+KeyOfMaxServiceID)
		serviceIDVal = "unknown (init error)"
	}

	if containerResp != nil && len(containerResp.Kvs) > 0 {
		containerIDVal = string(containerResp.Kvs[0].Value)
	} else {
		log.Printf("initUUIDData: MaxContainerID key ('%s') not found or value empty after GetNCreate.", s.cfg.Prefix+"/"+KeyOfMaxContainerID)
		containerIDVal = "unknown (init error)"
	}

	log.Printf("flake max_serviceid:%s, max_containerid:%s", serviceIDVal, containerIDVal)
	return true, nil
}

// StartServer create a server and run it.
func StartServer(cfg *Config) (*UUIDServer, error) {
	rand.Seed(time.Now().UnixNano())

	svr := &UUIDServer{cfg: cfg}
	log.Printf("flake config: %v", cfg)

	// init etcdclient
	etcdWrapCfg := &EtcdWrapConfig{
		Endpoints: svr.cfg.Endpoints,
		UserName:  svr.cfg.UserName,
		Password:  svr.cfg.Password,
	}

	var err error
	svr.etcdWrap, err = NewEtcdWrap(etcdWrapCfg)
	if err != nil {
		return nil, err
	}

	// svr.etcdWrap.GetVersion() was removed. NewEtcdWrap now handles initial connection check/logging.
	log.Printf("EtcdWrap initialized for UUIDServer.")

	// init uuid server
	_, err = svr.initUUIDData() // initUUIDData now returns (bool, error)
	if err != nil {
		return nil, err
	}

	// start gRpc service
	svr.listen, err = net.Listen("tcp", cfg.ListenAddress)
	if err != nil {
		return nil, err
	}
	log.Printf("flake listen on %v", cfg.ListenAddress)

	svr.grpcServer = grpc.NewServer()
	api.RegisterUUIDServer(svr.grpcServer, svr)
	svr.grpcServer.Serve(svr.listen)

	return svr, nil
}
