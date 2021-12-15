
# Libp2p benchmark

Build the docker image:

```
docker build -t yyyy .
```

Run agent 1:

```
$ docker run yyyy agent --bind-addr 127.0.0.1:3000 --proxy-addr '{{ GetInterfaceIP "eth0" }}:8000'
```

Run agent 2:

```
$ docker run yyyy agent --bind-addr 127.0.0.1:3000 --proxy-addr '{{ GetInterfaceIP "eth0" }}:8000'
```

Publish a message in agent 1:

```
$ curl 172.17.0.2:7000/publish
```

## Arguments

- **city**: Use a specific city, otherwise a random one is choosen from the latency matrix.

## API

Check the city of the agent:

```
$ curl 172.17.0.2:7000/system/city
Valencia
```

## Toxiproxy

If you change something from toxiproxy you might need to vendor again with:

```
$ go mod tidy
$ go mod vendor
```

since it is a dependency library.
