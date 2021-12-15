
# Libp2p benchmark

Run the agent:

```
$ go run main.go --bind-port 3000
```

Run a second agent:

```
$ go run main.go --bind-port 3001
```

They should find each other and connect over libp2p.

## Docker

```
$ go run main.go --bind-addr 127.0.0.1:3000 --nat '{{ GetInterfaceIP "eth0" }}'
```
