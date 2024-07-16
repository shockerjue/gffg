# gffg
The asynchronous RPC framework based on the protobuf protocol includes the interfaces of various components.It implements service registration, discovery, current limiting, and circuit breaking.The most important thing is that it can be developed secondary and expanded better. <br><br>
![Architecture](https://github.com/shockerjue/gffg/blob/master/docs/structure.png)
<br><br>
It can be expanded further and combined into a super application as follows.
![Architecture](https://github.com/shockerjue/gffg/blob/master/docs/platom.png)
<br><br><br>


## Service manage
Use polaris as the service registration center and management center. [polaris](https://github.com/polarismesh/polaris). The entire RPC service framework relies on Polaris, which serves as a route between services and is also used to provide service information.
``` docker run
docker run -d --privileged=true -p 15010:15010 -p 8101:8101 -p 8100:8100 -p 8080:8080 -p 8090:8090 -p 8091:8091 -p 8093:8093 -p 8761:8761 -p 8848:8848 -p 9848:9848 -p 9090:9090 -p 9091:9091 polarismesh/polaris-standalone:latest
```
![Polaris](https://github.com/shockerjue/gffg/blob/master/docs/polaris.png)
<br><br>
You can also use your own service management and implement the corresponding interface to use custom service registration and discovery.
```interface
type IRegistry interface {
	Provider(*node)
	Consumer()
	Register(string, string)
	Destroy()
	GetNode(context.Context, string, string) (NodeInstance, error)
	Limiter(context.Context, string) error
}
```
<br><br>

## Service Monitoring
Use grafana to monitor the framework service nodes and track the status. Report monitoring information by Kafka.
``` docker run
docker run -d --name=grafana -p 3000:3000 grafana/grafana-enterprise
```
<br><br>

## Monitoring and reporting service
Subscribe to the monitoring information of each service from Kafka, report the monitoring information to Promethums, and check the monitoring status of each node and interface.
- [metricsvr](https://github.com/gfzwh/metricsvr)
![grafana](https://github.com/shockerjue/gffg/blob/master/docs/metricsvr.png)
<br><br><br>


## protocol generate
The framework protocol uses protobuf. Once the .proto file is defined, the corresponding protocol file and service interface can be generated with one click.To generate protocols and interfaces, we rely on the following tools.

- [protoc](https://github.com/google/protobuf) [mac: brew install protobuf]
- [protoc-gen-gofast](https://github.com/gogo/protobuf/tree/master/protoc-gen-gofast)
- [protoc-gen-micro](https://github.com/shockerjue/protoc-gen-micro)
<br><br><br>

### Protocol File
```xxx.proto
syntax = "proto3";
package protocol;

message Request {
    string      username        = 1;
    string      telephone       = 2;
    string      email           = 3;
}

message Response {
    int32   Code                = 1;
    string  msg                 = 2;
    map<string,string> extra    = 3;
}


service XXXService {
    rpc  XxxCall(Request) returns (Response) {}
}
```
<br><br>

### Generate Commands
```
protoc -I. --gofast_out=. --micro_out=.  xxx.proto
```
<br><br>


### Generated Files
```
xxx.proto          struct and interface define
xxx.pb.go          Generate Service interface struct
xxx.pb.micro.go    Generate Service interface implement, client, server
```
<br><br>

## Example
- [Protocol Generation](https://github.com/shockerjue/gffg/tree/master/example/protocol) <br>
Define the .proto file and use the tool to generate the protocol file.

- [Server implement](https://github.com/shockerjue/gffg/tree/master/example/server) <br>
The rpc service mainly implements the interface defined by proto.

- [Client implement](https://github.com/shockerjue/gffg/tree/master/example/client)
<br><br>


## Performance Analysis
Performance analysis can be achieved using the Pprof method under tools, which can generate the corresponding function call graph and Firefox graph.How to use it:
```
package main

import (
  ...
  "github.com/shockerjue/gffg/tools"
)

func main() {
	...

	// Asynchronous Execution
	tools.PProf()
	...
	return
}

```
You can execute the following command to collect data. After collecting data, you can access the performance indicator data through http://127.0.0.1:7778.

```
go tool pprof -http=:7778 -seconds=20 http://localhost:7777/debug/pprof/profile
```

- [flame](https://github.com/shockerjue/gffg/blob/master/docs/flame.png)
![flame](https://github.com/shockerjue/gffg/blob/master/docs/flame.png)
- [pprof](https://github.com/shockerjue/gffg/blob/master/docs/pprof.png)
![pprof](https://github.com/shockerjue/gffg/blob/master/docs/pprof.png)
<br><br>
