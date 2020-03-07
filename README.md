# flake
![CI](https://github.com/cnwinds/flake/workflows/CI/badge.svg?branch=master)

flake是一个分布式ID生成算法的实现。他基于snowflake算法修改，使得在k8s环境下能更好的工作。他的主要特点：

* flake基于k8s微服务架构的微服务设计，可以部署多个服务端，避免服务端的单点故障。 数据存储使用k8s的etcd进行保存。
* 同一个业务启动多个微服务端，可以同时请求，保证每个端都能生成唯一ID。
* 算法中不使用时间，避免时间回拨的问题。

flake使用go语言编写。由分配UUID段的服务端和客户端库组成。服务端使用docker容器部署运行。

# Getting started

下面开始构建一个测试运行环境。在正式开始之前需要：

* 安装go开发环境 （[golang](https://golang.org/dl/)）
* k8s的运行环境 （[docker-desktop](https://www.docker.com/products/docker-desktop) ，[开启k8s的方法](https://github.com/AliyunContainerService/k8s-for-docker-desktop)）

完成以上工作后，使用以下命令进行验证，如果能看到对应的结果则说明准备环境已经搭建完成。

```
> go version
go version go1.13.8 windows/amd64
```

```
> kubectl version
Client Version: version.Info{Major:"1", Minor:"15", GitVersion:"v1.15.5", GitCommit:"20c265fef0741dd71a66480e35bd69f18351daea", GitTreeState:"clean", BuildDate:"2019-10-15T19:16:51Z", GoVersion:"go1.12.10", Compiler:"gc", Platform:"windows/amd64"}
Server Version: version.Info{Major:"1", Minor:"15", GitVersion:"v1.15.5", GitCommit:"20c265fef0741dd71a66480e35bd69f18351daea", GitTreeState:"clean", BuildDate:"2019-10-15T19:07:57Z", GoVersion:"go1.12.10", Compiler:"gc", Platform:"linux/amd64"}
```

## 构建镜像
1. 将项目拉取到本地。并进入项目目录。
```bash
git clone https://github.com/cnwinds/flake.git
cd flake
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
这里有两种方式启动服务端:

**1. docker方式**
```
docker-compose -f .\test_compose.yml up -d
```

**2. kubernetes方式**
```
kubectl create -f .\test_k8s.yaml
```

在测试环境里这两种方式结果是一样的，任选一种启动服务端。

部署成功后，etcd的服务端口映射到32379，32380。 flake的服务端口映射到本机的31000。

**注意**：测试环境的etcd没有挂载存储，所以每次重启后里面的数据都会丢失！

## 运行测试

```bash
go test -v
```

不出意外的话，应该可以看到测试结果了。

## 关闭服务端

如果不需要测试环境的容器则可以运行以下命令进行清理。

**1. docker方式**
```
docker-compose -f .\test_compose.yml down
```

**2. kubernetes方式**
```
kubectl delete -f .\test_k8s.yaml
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


## GO客户端集成
下面展示了go客户端里怎样集成flake库获取UUID
```golang
cfg := &client.Config{
    Endpoint:    "127.0.0.1:31000", // flake server address
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
flake返回的UUID是一个64bit的整数。由符号位，服务名ID，容器名ID，顺序号，一共4个部分组成。

bit63 | bit62...bit53 | bit52...bit31 | bit30...bit0
-|-|-|-
1-bit | (10bit)存放服务名ID | (22bit)存放容器名ID | (31bit)存放顺序号

* 最高的一个bit位控制符号，UUID都为正数，这里为0
* 存放服务名ID共10bit，可以有1024个数。客户端请求的时候服务名给的是字符串，flake服务端发现是没有注册过的名字则用服务名的自增唯一ID分配一个最大的ID并注册。下次请求时则能查到对应关系，使用已经注册的ID返回。
* 存放容器名ID共22bit，可以有4194304个数。每个启动的容器都会有一个唯一的容器名称。名字和ID的注册关系同服务名。即使相同的服务名，在不同的容器里运行也会使用不同的容器ID，这样可以尽可能避免对etcd中键值修改的冲突。
* 顺序号共31bit，最大值是21亿。如果出现超过上限的情况，则强行将容器ID重新分配一个新ID，这样就又可以有21亿个UUID了。

该算法中依赖的etcd存储并不支持事务操作，所以需要小心处理键值修改的冲突情况。
