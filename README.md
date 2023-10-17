# Restake-Go

Restake-Go is an implementation of the Restake Daemon in native golang. Restake-Go runs over gRPC and is opinionated about retrying. 

Restake-Go is provided at a beta-level. Tessellated uses this software in production, and you may too, but we make no warranties or guarantees. 

## Installation

Installing `restake-go` is easy. Simply clone the repository and run `make`. You'll need `go`, `git`, `make` and probably some other standard dev tools you already have installed.

```shell
# Get restake-go
$ git clone https://github.com/tessellated-io/restake-go/

# Install restake-go
$ cd restake-go
$ make install

# Find out what restake-go can do. 
$ restake-go --help
```

## Quick Start

// TODO: Sample config
Copy the sample config:
```shell
$ mkdir ~/.restake
$ cp config.sample.yml  ~/.restake/config.yml
```

Fill out the config, then run `restake-go`:

```shell
$ restake-go start
```

## Features

Restake-go offers a number of features over the restake go. 

TODO: Fill this out
- automatic discover
- retry
- parallel execution 
- tx inclusion polling
- gas normalization / auto discover

As well as the features you know and love
- healthchecks


## Configuration 

TODO 