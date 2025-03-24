# Prom++

Prom++ is a Prometheus with tsdb rewrited with C++ and many optimisations. Prom++ use in average 8x less RAM.

## Architecture overview

![Architecture overview](documentation/images/architecture.svg)

## Install

There are various ways of installing Prom++.

### Precompiled binaries

Precompiled binaries for released versions are available at
[release page](https://github.com/deckhouse/promppold/releases).
Using the latest production release binary
is the recommended way of installing Prom++.

### Docker images

Docker images are available on [Docker Hub](https://hub.docker.com/r/deckhouse/prompp/).

You can launch a Prom++ container for trying it out with

```bash
docker run --name prompp -d -p 127.0.0.1:9090:9090 deckhouse/prompp
```

Prom++ will now be reachable at <http://localhost:9090/>.

### Building from source

To build Prom++ from source code, you need [werf](https://werf.io).

Start by cloning the repository:

```bash
git clone https://github.com/deckhouse/prompp.git
cd prompp
```

Command
```bash
werf build --add-custom-tag=prompp
```
built docker image with Prom++ in it.

### Service discovery plugins

Prometheus is bundled with many service discovery plugins.
When building Prometheus from source, you can edit the [plugins.yml](./plugins.yml)
file to disable some service discoveries. The file is a yaml-formated list of go
import path that will be built into the Prometheus binary.

After you have changed the file, you
need to run `make build` again.

If you are using another method to compile Prometheus, `make plugins` will
generate the plugins file accordingly.

If you add out-of-tree plugins, which we do not endorse at the moment,
additional steps might be needed to adjust the `go.mod` and `go.sum` files. As
always, be extra careful when loading third party code.

## Getting started

Prom++ use same configs and api as Prometheus. Just download binary and run it instead of Prometheus. An example of the above configuration file can be found [here.](https://github.com/deckhouse/prompp/blob/pp/documentation/examples/prometheus.yml)

### Converting WAL

Prom++ use different WAL format but share historical blocks format. So to migrate data between Prom++ and Prometheus you can just convert WAL to historical blocks with `prompptool` from release.

To convert Prometheus WAL to historical blocks use
```bash
prompptool walvanilla --working-dir <path to prometheus data dir>
```

To convert Prom++ WAL to historical blocks use
```bash
prompptool walpp --working-dir <path to prometheus data dir>
```

## React UI Development

For more information on building, running, and developing on the React-based UI, see the React app's [README.md](web/ui/README.md).

## Contributing

Refer to [CONTRIBUTING.md](https://github.com/deckhouse/prompp/blob/pp/CONTRIBUTING.md)

## License

Apache License 2.0, see [LICENSE](https://github.com/deckhouse/prompp/blob/pp/LICENSE).
