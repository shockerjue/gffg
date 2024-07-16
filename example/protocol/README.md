# gfz-protoc
gfz protobuf gen


## Install
Install protoc-gen-gofast,protoc-gen-micro,protoco。

```
go install ~/go/pkg/mod/github.com/gogo/protobuf@v1.3.2/protoc-gen-gofast/main.go
go install github.com/shockerjue/protoc-gen-micro@v0.1.2
```

Also required:

- [protoc](https://github.com/google/protobuf) [mac: brew install protobuf]
- [protoc-gen-gofast](https://github.com/gogo/protobuf/tree/master/protoc-gen-gofast)
- [protoc-gen-micro](https://github.com/shockerjue/protoc-gen-micro)

## gen
```
protoc -I. --gofast_out=. --micro_out=.  usernode.proto
```

```
usernode.proto          struct and interface define
usernode.pb.go          Generate Service interface struct
usernode.pb.micro.go    Generate Service interface implement, client, server
```
