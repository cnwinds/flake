/*
Flake is a golang implementation of a distributed ID generation algorithm.
He made modifications based on the snowflake algorithm to make it work better in the k8s environment.
His main characteristics:

* flake's microservice design based on the k8s microservice architecture enables the deployment of multiple servers to avoid a single point of failure on the server side.
The data store is saved using the etcd of k8s.
* the same business starts multiple microservers, which can be requested at the same time to ensure that each end can generate a unique ID.
* time is not used in the algorithm to avoid the problem of time callback.

Flake is written in the golang.
Consists of the server and client libraries that assign the UUID segment.
The server is deployed and run using the docker container.
*/