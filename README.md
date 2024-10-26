# Delth

![delth logo](doc/logo.png)

## Description

`delth` stands for `delay health`.

`delth` tries to solve a simple problem that I'm encountering when dealing with _container orchestration_.

In an infrastructure with an external _Load Balancer_ (`traefik`, etc...) and an _orchestrator_ (`docker swarm`, etc...), when rolling out new releases or updates, I've encountered scenarios where the _Load Balancer_ can still route requests to a container that is being shut down by the orchestrator.

The idea behind `delth` is quite simple. `delth` replaces your container `ENTRYPOINT` and:
* starts your _initial container entrypoint_
* starts an HTTP server to respond to your _Load Balancer_ health check
* sets up _signal handlers_ to add delay logic and _health check_ modification

### Initial ENTRYPOINT and signals

`delth` just starts your initial `ENTRYPOINT` in a dedicated _Session_.

`delth` will receive your orchestrator _signal_ to stop. This signal will **not** be immediatly be received by the _original `ENTRYPOINT`_ and your program will continue its normal operation for a while.

After a configurable delay, `delth` will _forward the signal_ to your _original `ENTRYPOINT`_ and only then, your program will stop its operation.

### Health check and signals

In normal operations, `delth` HTTP server _forwards_ the _Load Balancer_ Health check request to your _original `ENTRYPOINT`_.

**As soon** as `delth` receives a _signal to stop_, it responds with `HTTP 503 Service Unavailable` to the _health check_ requests.

This will be used by your _Load Balancer_ to know that this _container_ should no longer receive requests.

### All together

When a signal arrives:
* your next _Load Balancer_ health check request will fail and your _Load Balancer_ should remove the _container_ from the pool / stop routing requests to the _container_.
* within a configurable `delay`, your program will continue its normal operation and process current requests as usual.

It the _Load Balancer_ health check interval and the `delth` signal propagation `delay` are properly configured, your program should be able to terminate the requests it was doing and, very important, no longer receive new ones.

### With VS Without `delth` rolling update

This data is from the `./examples/sample-app` using `docker swarm` and `traefik`.

A `docker service update --force $SERVICE_NAME` is performed in both cases.

[vegeta](https://github.com/tsenart/vegeta) is used to _load test_ the application and observe potential failures.

#### Without `delth`

```
❯ echo "GET http://app-without-delth.127.0.0.1.nip.io" | vegeta attack -duration=0 -connections=4000 -rate 200/1s | tee /tmp/results.bin | vegeta report
^CRequests      [total, rate, throughput]         21755, 192.27, 185.77
Duration      [total, attack, wait]             1m53s, 1m53s, 36.38ms
Latencies     [min, mean, 50, 90, 95, 99, max]  270.037µs, 4.337s, 4.003s, 8.004s, 9.002s, 9.003s, 9.035s
Bytes In      [total, mean]                     2215749, 101.85
Bytes Out     [total, mean]                     0, 0.00
Success       [ratio]                           96.65%
Status Codes  [code:count]                      200:21026  502:729  
Error Set:
502 Bad Gateway
```

#### With `delth`

```
❯ echo "GET http://app-with-delth.127.0.0.1.nip.io" | vegeta attack -duration=0 -connections=4000 -rate 200/1s | tee /tmp/results-with-delth.bin | vegeta report
^CRequests      [total, rate, throughput]         32113, 194.37, 194.32
Duration      [total, attack, wait]             2m45s, 2m45s, 41.838ms
Latencies     [min, mean, 50, 90, 95, 99, max]  290.626µs, 4.455s, 4.003s, 8.369s, 9.002s, 9.003s, 9.049s
Bytes In      [total, mean]                     8798962, 274.00
Bytes Out     [total, mean]                     0, 0.00
Success       [ratio]                           100.00%
Status Codes  [code:count]                      200:32113  
Error Set:
```

🎉 no `HTTP 502` 🎉

## Usage

### Add `delth` to your container

In your `Dockerfile`, just adds a _stage_ such as:

```dockerfile
[...]

FROM alpine:latest AS delth-fetcher

ARG DELTH_VERSION=""

RUN test -n "${DELTH_VERSION}"
RUN apk add --no-cache curl && \
cd / && \
echo "Fetching riton/delth version '${DELTH_VERSION}'" && \
curl -fLO https://github.com/riton/delth/releases/download/v${DELTH_VERSION}/delth_Linux_x86_64.tar.gz && \
tar xvf delth_Linux_x86_64.tar.gz delth

[...]

##
# Your final image
##
FROM ...

COPY --from=delth-fetcher --chmod=0755 --chown=root:root /delth /usr/bin/delth

ENTRYPOINT ["/usr/bin/delth", "--", "$PUT_HERE_YOUR_ORIGINAL_ENTRYPOINT"]
```

and remember to replace `$PUT_HERE_YOUR_ORIGINAL_ENTRYPOINT` by the _absolute path_ to your `ENTRYPOINT` before you were using `delth`.

That's it, your container will now embed `delth` and it will do its magic 🪄.

### Configure `delth`

`delth` is configured using _environment variables_.

#### Health Check proxy configuration

**Mandatory parameters** `DELTH_BACKEND_HEALTHCHECK_PORT` and `DELTH_BACKEND_HEALTHCHECK_PATH` are used to configure the **port** and **path** where your application HTTP Health check is reachable.
`delth` will forward your _Load balancer_ Health check requests to this endpoint.

`DELTH_BACKEND_HEALTHCHECK_SCHEME` is used to specify if `http` or `https` HTTP scheme should be used to reach your application HTTP Health check. `DELTH_BACKEND_HEALTHCHECK_TLS_INSECURE_SKIP_VERIFY` can be used to ignore TLS errors if `DELTH_BACKEND_HEALTHCHECK_SCHEME` is set to `https` and you're using a self-signed certificate.

`DELTH_BACKEND_HEALTHCHECK_TIMEOUT` can be used to specify the HTTP timeout used to forward the health check requests to your application.

#### Command execution and signal propagation

`DELTH_CMD_EXEC_SHUTDOWN_DELAY` is the `delay` that `delth` will wait before it propagate the _stop signal_ to your application.

#### `delth` HTTP server

`DELTH_HEALTHCHECK_PROXY_LISTEN_ADDR` controls the _listen address_ used by `delth` HTTP server to listen for Health Check requests.

#### Default configuration

```yaml
DELTH_BACKEND_HEALTHCHECK_SCHEME: 'http'
DELTH_BACKEND_HEALTHCHECK_TLS_INSECURE_SKIP_VERIFY: 0
DELTH_BACKEND_HEALTHCHECK_TIMEOUT: '30s'
DELTH_CMD_EXEC_SHUTDOWN_DELAY: '30s'
DELTH_HEALTHCHECK_PROXY_LISTEN_ADDR: ':8069'
```
