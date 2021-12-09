
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
