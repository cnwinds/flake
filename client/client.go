package client

import (
	"context"
	"sync"

	"github.com/cnwinds/flake/api"
	"github.com/cnwinds/flake/util"

	"google.golang.org/grpc"
)

type Config struct {
	Endpoint    string
	IsPrevFetch bool
	NeedCount   int
}

type uuidNode struct {
	datas       []*api.UUIDRange
	takeLock    sync.Mutex
	fetchLock   sync.Mutex
	isFetching  bool
	needCount   int
	leftCount   int
	serviceName string
}

type Client struct {
	cfg  *Config
	conn *grpc.ClientConn
	api  api.UUIDClient

	containerName string

	storeLock sync.Mutex
	store     map[string]*uuidNode
}

func (c *Client) fetch(serviceName string, containerName string, needCount int) (*api.FetchReply, error) {
	resp, err := c.api.Fetch(context.Background(), &api.FetchRequest{ServiceName: serviceName, ContainerName: containerName,
		NeedCount: int32(needCount)})
	return resp, err
}

func (c *Client) SetNeedCount(serviceName string, needCount int) {
	key := serviceName + c.containerName

	c.storeLock.Lock()
	defer c.storeLock.Unlock()

	v, ok := c.store[key]
	if ok == false {
		c.store[key] = &uuidNode{needCount: c.cfg.NeedCount}
	}
	v, ok = c.store[key]
	v.needCount = needCount
}

func (c *Client) GenUUID(serviceName string) (uuid int64, err error) {
	key := serviceName + c.containerName

	c.storeLock.Lock()
	v, ok := c.store[key]
	if ok == false {
		c.store[key] = &uuidNode{needCount: c.cfg.NeedCount}
		v, ok = c.store[key]
	}
	c.storeLock.Unlock()

	for {
		v.takeLock.Lock()
		if len(v.datas) > 0 {
			r := v.datas[0]
			v1 := r.ServiceId
			v2 := r.ContainerId
			v3 := r.SequenceIdStart
			r.SequenceIdStart++
			if r.SequenceIdStart > r.SequenceIdEnd {
				// remove used data
				v.datas = v.datas[1:]
			}
			v.leftCount--

			// fetch data in advance
			if c.cfg.IsPrevFetch {
				if v.isFetching == false && v.leftCount < v.needCount/2 {
					// start coroutines
					v.isFetching = true
					go c.fetchAndInsert(serviceName, v)
				}
			}
			v.takeLock.Unlock()

			// return uuid
			return util.GenUUID(v1, v2, v3), nil
		}
		v.takeLock.Unlock()

		// fetch data
		err = c.fetchAndInsert(serviceName, v)
		if err != nil {
			return 0, err
		}
	}
}

func (c *Client) fetchAndInsert(serviceName string, node *uuidNode) error {
	node.fetchLock.Lock()
	defer node.fetchLock.Unlock()

	node.takeLock.Lock()
	if len(node.datas) > 0 {
		node.isFetching = false
		node.takeLock.Unlock()
		return nil
	}
	node.isFetching = true
	node.takeLock.Unlock()

	needCount := node.needCount
	resp, err := c.fetch(serviceName, c.containerName, needCount)
	if err != nil {
		return err
	}
	node.takeLock.Lock()
	node.datas = append(node.datas, resp.Items...)
	node.leftCount += needCount
	node.isFetching = false
	node.takeLock.Unlock()

	return nil
}

func (c *Client) Close() {
	c.conn.Close()
	c.store = nil
}

func NewClient(cfg *Config) (client *Client, err error) {
	client = &Client{cfg: cfg}
	if client.cfg.NeedCount == 0 {
		client.cfg.NeedCount = 1000
	}
	client.conn, err = grpc.Dial(client.cfg.Endpoint, grpc.WithInsecure())
	if err != nil {
		return nil, err
	}
	client.api = api.NewUUIDClient(client.conn)
	client.containerName = util.GetContainerName()
	client.store = make(map[string]*uuidNode)
	return client, nil
}
