package server

import (
	"fmt"
	"log"
	"math/rand"
	"net"
	"strconv"
	"time"

	"github.com/cnwinds/flake/api"

	"github.com/coreos/etcd/client"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
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
	serviceID := 0
	for {
		r, err := s.etcdWrap.Get(key)
		if err != nil {
			if s.etcdWrap.IsKeyNotFound(err) {
				if serviceID == 0 {
					serviceID, err = s.nextServiceID()
					if err != nil {
						return 0, err
					}
				}
				resp, err := s.etcdWrap.Set(key, strconv.Itoa(serviceID), &client.SetOptions{PrevExist: "false"})
				if err != nil {
					// create conflict, again
					continue
				}
				// create success
				return strconv.Atoi(resp.Node.Value)
			}
			return 0, nil
		}
		// get success
		return strconv.Atoi(r.Node.Value)
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
	containerID := 0
	for {
		r, err := s.etcdWrap.Get(key)
		if err != nil {
			if s.etcdWrap.IsKeyNotFound(err) {
				if containerID == 0 {
					containerID, err = s.nextContainerID()
					if err != nil {
						return 0, err
					}
				}
				resp, err := s.etcdWrap.Set(key, strconv.Itoa(containerID), &client.SetOptions{PrevExist: "false"})
				if err != nil {
					// create conflict, again
					continue
				}
				return strconv.Atoi(resp.Node.Value)
			}
			return 0, err
		}
		return strconv.Atoi(r.Node.Value)
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
	containerID, err := s.nextContainerID()
	if err != nil {
		return err
	}
	for {
		r, err := s.etcdWrap.Get(key)
		if err != nil {
			return err
		}
		_, err = s.etcdWrap.Set(key, strconv.Itoa(containerID), &client.SetOptions{PrevIndex: r.Node.ModifiedIndex})
		if err != nil {
			// modify conflict, again
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
		resp, err := s.etcdWrap.Get(key)
		if err != nil {
			if s.etcdWrap.IsKeyNotFound(err) {
				startID = 1
				endID = startID + needCount
				if endID > MaxOfSequence {
					err := s.ReassignContainerID(containerName)
					if err != nil {
						return 0, 0, 0, 0, err
					}
					endID = MaxOfSequence
				}
				resp, err = s.etcdWrap.Set(key, strconv.Itoa(endID), &client.SetOptions{PrevExist: "false"})
				if err != nil && endID != MaxOfSequence {
					// create conflict, again
					continue
				}
				// create success
				return serviceID, containerID, startID, endID - 1, nil
			}
			return 0, 0, 0, 0, err
		}

		startID, err := strconv.Atoi(resp.Node.Value)
		if err != nil {
			return 0, 0, 0, 0, err
		}

		if startID == MaxOfSequence {
			// deadlock prevention
			err := s.ReassignContainerID(containerName)
			if err != nil {
				return 0, 0, 0, 0, err
			}
			// container id reassigned, relaunch function
			return s.getUUIDSegment(serviceName, containerName, needCount)
		}

		endID = startID + needCount
		if endID > MaxOfSequence {
			err := s.ReassignContainerID(containerName)
			if err != nil {
				return 0, 0, 0, 0, err
			}
			endID = MaxOfSequence
		}
		resp, err = s.etcdWrap.Set(key, strconv.Itoa(endID), &client.SetOptions{PrevIndex: resp.Node.ModifiedIndex})
		if err != nil {
			// modify conflict, again
			continue
		}
		// modify success
		return serviceID, containerID, startID, endID - 1, nil
	}
}

func (s *UUIDServer) initUUIDData() (success bool, err error) {
	serviceResp, err := s.etcdWrap.GetNCreate(s.cfg.Prefix+"/"+KeyOfMaxServiceID, StartOfServerID)
	if err != nil {
		return false, err
	}
	containerResp, err := s.etcdWrap.GetNCreate(s.cfg.Prefix+"/"+KeyOfMaxContainerID, StartOfContainerID)
	if err != nil {
		return false, err
	}

	log.Printf("flake max_serviceid:%v, max_containerid:%v", serviceResp.Node.Value, containerResp.Node.Value)
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

	ver, err := svr.etcdWrap.GetVersion()
	if err != nil {
		return nil, err
	}
	log.Printf("etcd version: %v", ver)

	// init uuid server
	svr.initUUIDData()
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
