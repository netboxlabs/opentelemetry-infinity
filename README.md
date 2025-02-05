<img src="docs/images/logo.png" align="left" width="80px"/>

# OpenTelemetry Infinity

<br clear="left"/>

[![Build status](https://github.com/netboxlabs/opentelemetry-infinity/actions/workflows/build.yaml/badge.svg)](https://github.com/netboxlabs/opentelemetry-infinity/actions)
[![CodeQL](https://github.com/netboxlabs/opentelemetry-infinity/workflows/CodeQL/badge.svg)](https://github.com/netboxlabs/opentelemetry-infinity/security/code-scanning)
[![Go Report Card](https://goreportcard.com/badge/github.com/netboxlabs/opentelemetry-infinity)](https://goreportcard.com/report/github.com/netboxlabs/opentelemetry-infinity)
![GitHub release (latest by date)](https://img.shields.io/github/v/release/netboxlabs/opentelemetry-infinity)

<p align="left">
  <strong>
  <a href="#what-is-it">What is it</a>&nbsp;&nbsp;&bull;&nbsp;&nbsp;
    <a href="#project-premises">Premises</a>&nbsp;&nbsp;&bull;&nbsp;&nbsp;
    <a href="#command-line-interface-cli">Command Line Interface</a>&nbsp;&nbsp;&bull;&nbsp;&nbsp;
    <a href="#rest-api">REST API</a>&nbsp;&nbsp;&bull;&nbsp;&nbsp;
    <a href="#policy-rfc-v1">Policy RFC</a>
  </strong>
</p>

---

## What is it
Opentelemetry Infinity provide [otel-collector-contrib](https://github.com/open-telemetry/opentelemetry-collector-contrib) instances through a simple REST API using a policy mechanism. Each policy spin up a new `otelcol-contrib` process running the configuration provided by the policy.

## Project premises
**1. Single binary**: `otlpinf` embeds `otelcol-contrib` in its binary. Therefore, only one static binary is provided.

**2. No persistence**: `opentelemetry-infinity` stores data in memory and in temporary files only. This adds a new paradigm to `opentelemetry-collector` that is expected to run over a persisted config file as default. If you are looking for a opentelemetry orchestrator as the way it was planned to perform, you should try the official [opentelemetry-operator](https://github.com/open-telemetry/opentelemetry-operator).

**3. Compatibility**: `opentelemetry-infinity` is basically a wrapper over the official `opentelemetry-collector` which has not released a version `1.0` yet, i.e., breaking changes are expected. Any changes that occurs on its CLI will be reflected in this project.

**4. Versioning**: as `opentelemetry-infinity` is a wrapper over the official collector, it follows the official `opentelemetry-collector` version. `opentelemetry-infinity` pipeline does  releases automatically on every new `otelcol-contrib`. 

## Docker Image
You can download and run using docker image:
```
docker run --net=host netboxlabs/opentelemetry-infinity run
```
## Command Line Interface (CLI)
Opentelemetry Infinity allows some start up configuration that is listed below. It disables `opentelemetry-collector` self telemetry by default to avoid port conflict. If you want to enable it back, be aware to handle it properly when starting more that one `otelcol-contrib`, i.e., applying more than one policy.
```sh
docker run --net=host netboxlabs/opentelemetry-infinity run --help

Run opentelemetry-infinity

Usage:
  opentelemetry-infinity run [flags]

Flags:
  -d, --debug                  Enable verbose (debug level) output
  -f, --feature_gates string   Define opentelemetry feature gates
  -h, --help                   help for run
  -s, --self_telemetry         Enable self telemetry for collectors. It is disabled by default to avoid port conflict
  -a, --server_host string     Define REST Host (default "localhost")
  -p, --server_port uint       Define REST Port (default 10222)
  -e, --set strings            Define opentelemetry set
```


## REST API
The default `otlpinf` address is `localhost:10222`. to change that you can specify host and port when starting `otlpinf`:
```sh
docker run --net=host netboxlabs/opentelemetry-infinity run -a {host} -p {port}
```

### Routes (v1)
`otlpinf` is aimed to be simple and straightforward. 

#### Get runtime and capabilities information

<details>
 <summary><code>GET</code> <code><b>/api/v1/status</b></code> <code>(gets otlpinf runtime data)</code></summary>

##### Parameters

> None

##### Responses

> | http code     | content-type                      | response                                                            |
> |---------------|-----------------------------------|---------------------------------------------------------------------|
> | `200`         | `application/json; charset=utf-8` | JSON data                                                           |

##### Example cURL

> ```javascript
>  curl -X GET -H "Content-Type: application/json" http://localhost:10222/api/v1/status
> ```

</details>

<details>
 <summary><code>GET</code> <code><b>/api/v1/capabilities</b></code> <code>(gets otelcol-contrib capabilities)</code></summary>

##### Parameters

> None

##### Responses

> | http code     | content-type                      | response                                                            |
> |---------------|-----------------------------------|---------------------------------------------------------------------|
> | `200`         | `application/json; charset=utf-8` | JSON data                                                           |

##### Example cURL

> ```javascript
>  curl -X GET -H "Content-Type: application/json" http://localhost:10222/api/v1/capabilities
> ```

</details>

#### Policies Management

<details>
 <summary><code>GET</code> <code><b>/api/v1/policies</b></code> <code>(gets all existing policies)</code></summary>

##### Parameters

> None

##### Responses

> | http code     | content-type                      | response                                                            |
> |---------------|-----------------------------------|---------------------------------------------------------------------|
> | `200`         | `application/json; charset=utf-8` | JSON array containing all applied policy names                      |

##### Example cURL

> ```javascript
>  curl -X GET -H "Content-Type: application/json" http://localhost:10222/api/v1/policies
> ```

</details>


<details>
 <summary><code>POST</code> <code><b>/api/v1/policies</b></code> <code>(Creates a new policy)</code></summary>

##### Parameters

> | name      |  type     | data type               | description                                                           |
> |-----------|-----------|-------------------------|-----------------------------------------------------------------------|
> | None      |  required | YAML object             | yaml format specified in [Policy RFC](#policy-rfc-v1)                 |
 

##### Responses

> | http code     | content-type                       | response                                                            |
> |---------------|------------------------------------|---------------------------------------------------------------------|
> | `201`         | `application/x-yaml; charset=UTF-8`| YAML object                                                         |
> | `400`         | `application/json; charset=UTF-8`  | `{ "message": "invalid Content-Type. Only 'application/x-yaml' is supported" }`|
> | `400`         | `application/json; charset=UTF-8`  | Any policy error                                                    |
> | `400`         | `application/json; charset=UTF-8`  | `{ "message": "only single policy allowed per request" }`           |
> | `403`         | `application/json; charset=UTF-8`  | `{ "message": "config field is required" }`                         |
> | `409`         | `application/json; charset=UTF-8`  | `{ "message": "policy already exists" }`                            |
 

##### Example cURL

> ```javascript
>  curl -X POST -H "Content-Type: application/x-yaml" --data @post.yaml http://localhost:10222/api/v1/policies
> ```

</details>

<details>
 <summary><code>GET</code> <code><b>/api/v1/policies/{policy_name}</b></code> <code>(gets information of a specific policy)</code></summary>

##### Parameters

> | name              |  type     | data type      | description                         |
> |-------------------|-----------|----------------|-------------------------------------|
> |   `policy_name`   |  required | string         | The unique policy name              |

##### Responses

> | http code     | content-type                        | response                                                            |
> |---------------|-------------------------------------|---------------------------------------------------------------------|
> | `200`         | `application/x-yaml; charset=UTF-8` | YAML object                                                         |
> | `404`         | `application/json; charset=UTF-8`   | `{ "message": "policy not found" }`                                 |

##### Example cURL

> ```javascript
>  curl -X GET http://localhost:10222/api/v1/policies/my_policy
> ```

</details>

<details>
 <summary><code>DELETE</code> <code><b>/api/v1/policies/{policy_name}</b></code> <code>(delete a existing policy)</code></summary>

##### Parameters

> | name              |  type     | data type      | description                         |
> |-------------------|-----------|----------------|-------------------------------------|
> |   `policy_name`   |  required | string         | The unique policy name              |

##### Responses

> | http code     | content-type                      | response                                                            |
> |---------------|-----------------------------------|---------------------------------------------------------------------|
> | `200`         | `application/json; charset=UTF-8` | `{ "message": "my_policy was deleted" }`                            |
> | `404`         | `application/json; charset=UTF-8` | `{ "message": "policy not found" }`                                 |

##### Example cURL

> ```javascript
>  curl -X DELETE http://localhost:10222/api/v1/policies/my_policy
> ```

</details>

## Policy RFC (v1)

```yaml
my_policy:
  receivers:
    otlp:
      protocols:
        http:
        grpc:

  exporters:
    debug:

  service:
    pipelines:
      metrics:
        receivers:
        - otlp
        exporters:
        - debug
```
