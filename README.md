# flake
flake是一个分布式ID生成算法的实现。他基于flake算法修改，使得在k8s环境下能更好的工作。他的主要特点：

* flake基于k8s微服务架构的微服务设计，可以部署多个服务端，避免服务端的单点故障。 数据存储使用k8s的etcd进行保存。
* 同一个业务启动多个微服务端，可以同时请求，保证每个端都能生成唯一ID。
* 算法中不使用时间，避免时间回拨的问题。

flake使用go语言编写。由分配UUID段的服务端和客户端库组成。服务端使用docker容器部署运行。

# Getting started

下面开始构建一个用于测试环境的运行环境。
## 构建服务端
1. 将项目拉取到本地。
```bash
git clone https://github.com/cnwinds/flake.git
```

2. 获取go的依赖库。
```bash
go mod tidy
go mod vendor

```
3. 构建服务端镜像。
```bash
docker build -t flake:v1 .
```

## 运行服务端

1. 启动测试用的etcd
```bash
docker run -d -p32379:2379 -p32380:2380 xieyanze/etcd3:latest
```
*注意*： 以上命令行启动的etcd没有挂载存储，每次重启后数据会丢失。

2. 启动flake服务端
```bash
go run flake.go
```

## 运行测试

```bash
go test -v
```

# 术语
## UUID段
在flake中生成一个UUID的时候不是每次都要向flake服务端请求，这样效率太低。每次请求flake服务端的时候都会获取一定数量的UUID，然后客户端可以自己分配，直到全部用完。每次获取一定数量的UUID称之为UUID段。

## 预获取
客户端每次通过网络获取时需要一定的时间，预获取就是提前通过服务端获取一个UUID段，减少网络调用带来的延迟。

## 服务名
在获取UUID的时候需要提供一个服务名区别不同的业务。反映在UUID中将得到不同数值范围。

## 容器名
运行业务服务时一个容器运行环境的唯一名称。这里对应docker运行镜像时的CONTAINER ID。每次启动一个容器都会得到一个新的容器名。


## 获取UUID
下面展示了go客户端里怎样集成flake库获取UUID
```golang
	cfg := &client.Config{
		Endpoint:    "127.0.0.1:30001", // flake server address
		IsPrevFetch: true}              // 预获取
	c, err := client.NewClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

    c.SetNeedCount("User", 1000)       // 设置UUID段的大小
```

以上是初始化部分。接下来就可以生成UUID了。

```golang
	v, err := c.GenUUID("User")
	if err != nil {
		log.Println(err)
		os.Exit(1)
	}
	log.Printf("UUID value: %v", v)
```

也可以同时给多个业务系统的分配UUID。

```golang
    c.SetNeedCount("Order", 10000)  // 设置UUID段的大小

    v, err := c.GenUUID("Order")
	if err != nil {
		log.Println(err)
		os.Exit(1)
	}
	log.Printf("UUID value: %v", v)
```

# flake算法
flake返回的UUID是一个64bit的整数，最高位固定为0。 10bit用来表示不同服务名。22bit用来表示不同的运行容器。31bit表示对应的顺序UUID值。最终拼接成唯一的UUID。

```
0	    0........0          0....................0  0.............................0
1-bit   10bit service id    22bit container id      31bit sequence
```
其中服务名和容器名我们提供的时字符串，但是在UUID中要变成一个整数。
这个过程是由flake服务端完成。flake在etcd中保存了一个每个服务名和id的对应关系。首次查找没有这个对应关系则分配一个自增的唯一ID。同理容器名也是这样分配唯一ID。

对于UUID的顺序值部分，目前的最大值是21亿。如果出现超过上限的情况，则将容器ID重新分配一个新值，这样就又可以有21亿个UUID了。

该算法中依赖的etcd存储并不支持事务操作，所以需要小心处理键值修改的冲突情况。
