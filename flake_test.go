package main

import (
	"log"
	"sync"
	"testing"
	"time"

	"github.com/cnwinds/flake/client"
)

// verify that the generated uuid is duplicated
type verify struct {
	storeMap    map[int64]int
	dupMap      map[int64]int
	verifyQueue chan int64
	done        sync.WaitGroup
}

func (v *verify) Start() {
	v.storeMap = make(map[int64]int, 10000)
	v.dupMap = make(map[int64]int, 10000)
	v.verifyQueue = make(chan int64, 100000)
	f := func() {
		for true {
			value := <-v.verifyQueue
			if value > 0 {
				s, ok := v.storeMap[value]
				if ok == true {
					v.storeMap[value] = s + 1
					v.dupMap[value] = s + 1
				} else {
					v.storeMap[value] = 1
				}
			} else {
				v.done.Done()
				break
			}
		}
	}
	v.done.Add(1)
	go f()
}

func (v *verify) Verify(value int64) {
	v.verifyQueue <- value
}

func (v *verify) Stop() {
	v.verifyQueue <- -1
	v.done.Wait()
}

func (v *verify) HasError() (bool, map[int64]int) {
	if len(v.dupMap) > 0 {
		return true, v.dupMap
	}
	return false, nil
}

func TestNormal(t *testing.T) {
	// t.SkipNow()

	cfg := &client.Config{Endpoint: "127.0.0.1:31000", IsPrefetch: true}
	c, err := client.NewClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	ve := new(verify)
	ve.Start()

	key := "TestNormal"
	c.SetNeedCount(key, 1000)
	for i := 0; i < 100000; i++ {
		v, err := c.GenUUID(key)
		if err != nil {
			t.Fatal(err)
		}

		ve.Verify(v)

		if i%10000 == 0 {
			log.Printf("TestNormal: Complete count: %v, uuid value: %v", i, v)
		}
	}

	ve.Stop()
	r, dupMap := ve.HasError()
	if r == true {
		log.Fatalf("%v", dupMap)
	}
}

func TestPreFetchEffect(t *testing.T) {
	// t.SkipNow()

	// the service call speed is very fast, and the advantages are not obvious.
	t1 := time.Now()
	{
		cfg := &client.Config{Endpoint: "127.0.0.1:31000", IsPrefetch: true}
		c, err := client.NewClient(cfg)
		if err != nil {
			t.Fatal(err)
		}
		defer c.Close()

		key := "TestPreFetchEffect"
		c.SetNeedCount(key, 10000)
		for i := 0; i < 1000000; i++ {
			_, err := c.GenUUID(key)
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	log.Printf("have prefetch cost time: %v", time.Since(t1))

	t2 := time.Now()
	{
		cfg := &client.Config{Endpoint: "127.0.0.1:31000", IsPrefetch: false}
		c, err := client.NewClient(cfg)
		if err != nil {
			t.Fatal(err)
		}
		defer c.Close()

		key := "TestPreFetchEffect"
		c.SetNeedCount(key, 10000)
		for i := 0; i < 1000000; i++ {
			_, err := c.GenUUID(key)
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	log.Printf("no prefetch cost time: %v", time.Since(t2))
}

func TestOverSegment(t *testing.T) {
	t.SkipNow()
	// important:
	// change "MaxOfSequence = 1 << 10" to "MaxOfSequence = 1 << 10" in "uuid_server.go" file, and complete the test
	cfg := &client.Config{Endpoint: "127.0.0.1:31000", IsPrefetch: true}
	c, err := client.NewClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	key := "TestOverSegment"
	c.SetNeedCount(key, 2049)
	for i := 0; i < 65535; i++ {
		v, err := c.GenUUID(key)
		if err != nil {
			t.Fatal(err)
		}
		if i%5000 == 0 {
			log.Printf("Complete count: %v, uuid value: %v", i, v)
		}
	}
}

func TestParallel1(t *testing.T) {
	// t.SkipNow()

	cfg := &client.Config{Endpoint: "127.0.0.1:31000", IsPrefetch: false}
	c, err := client.NewClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	key := "TestParallel"
	c.SetNeedCount(key, 10000)

	var w sync.WaitGroup
	f := func(index int) {
		for i := 0; i < 1000000; i++ {
			_, err := c.GenUUID(key)
			if err != nil {
				t.Fatal(err)
			}
		}
		w.Done()
	}

	w.Add(4)
	go f(1)
	go f(2)
	go f(3)
	go f(4)
	w.Wait()
}

func TestParallel2(t *testing.T) {
	// t.SkipNow()

	cfg := &client.Config{Endpoint: "127.0.0.1:31000", IsPrefetch: true}
	c, err := client.NewClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	var w sync.WaitGroup
	f := func(key string) {
		for i := 0; i < 1000000; i++ {
			_, err := c.GenUUID(key)
			if err != nil {
				t.Fatal(err)
			}
		}
		w.Done()
	}

	keys := []string{"TestParallel1", "TestParallel2", "TestParallel3", "TestParallel4"}
	needCounts := []int{10000, 10000, 10000, 10000}

	w.Add(len(keys))
	for i := 0; i < len(keys); i++ {
		c.SetNeedCount(keys[i], needCounts[i])
		go f(keys[i])
	}
	w.Wait()
}
