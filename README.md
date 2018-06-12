# CarrIOTA Hercules

Hercules is a lightweight alternative to IOTA's IRI. The main advantage is that it compiles
to native code and does not need Java Virtual Machine, which considerably decreases the amount
of resources needed while increasing the performance.

This way, Hercules can run on low-end devices such as Raspberry PI.

Another feature that goes beyond IRI is local snapshots that can be done automatically
or through the API for custom snapshotting strategies. Next step is to allow certain transactions,
bundles, addresses and tags to be persisted and kept beyond the snapshots.

Intrigued? Give it a try!

## Requirements

Depending on how much of tangle history you want to keep, you will need different amount of free
space available on your drive. We suggest at least 1GB per day of stored tangle history.
THis value might increase as transaction speed changes in the future.
The compiled Hercules binary is just 20MB.

Regarding RAM: we officially support devices with at least 1GB RAM. For anything below 2-3GB, please 
make sure to use the `light` option when starting hercules.

Hercules communicates over UDP with other IRI and Hercules nodes. This means that if you are
behind a proxy or a router, you will need to setup a rule to allow connections to that port.
The default UDP port is `14600`.

## Building from scratch

These instructions will get you a copy of the project up and running on your local machine.
If you have downloaded the binaries, you can skip this part.

The following instructions are for linux. Linux and Windows might deviate a little.

### Install Go

You will need the latest [Golang](https://golang.org/dl/) (1.10.3+).

```
# Make sure to download the correct package for your OS. Raspberry is another one, for example.
wget https://dl.google.com/go/go1.10.3.linux-amd64.tar.gz

# Unpack and move to the correct location:
tar -xvf  go1.10.3.linux-amd64.tar.gz
sudo mv go /usr/local
```

**UBUNTU default Go package**: Be aware that Ubuntu usually comes with an outdated version. You can check what version
your go is with `go version` command.

You need to specify `GOROOT`, `GOPATH` and adjust `PATH` to include the new go directory.
Simply add these to your `.profile`:

```
export GOROOT=/usr/local/go
export GOPATH=$HOME/go
export PATH=$GOPATH/bin:$GOROOT/bin:$PATH
```

Now run `go version` to make sure that everything works and you have the correct version installed.

### Get Hercules

Simply run:

```
go get gitlab.com/semkodev/hercules
```

This will download and place the hercules source files in `$HOME/go/gitlab.com/semkodev/hercules`.

Now you can build Hercules with go. In the example below it is supposed, that you have
a `build` directory in your home folder, where the binary is placed. Make sure to
change it accordingly, if you want it to be compiled somewhere else:

```
go build -v -o ~/build/hercules ./go/src/gitlab.com/semkodev/hercules/hercules.go
```

That's it! Hercules is ready to be used.

## Running hercules

Running Hercules can be as easy as `./hercules`. For devices with very limited, please check 
[Running on low-end devices](#running-on-low-end-devices).

Depending on your OS, you might want to create a service or a startup script to run hercules
in the background.

You can configure Hercules using:

1. Config file
2. Environment variables (higher priority than the config file)
3. Command line parameters (higher priority than environment variables)

There are several configuration options available for hercules. They are structured hierarchically.
For example, setting api authentication username can be done in a JSON config file:

```
{
  "api": {
    "auth": {
      "username": "max"
    }
  }
}
```

The environment variables are prepended with `HERCULES_` and are written **UPPERCASE**.
The hierarchical levels use underscore for separation.
The example above as environment var: `HERCULES_API_AUTH_USERNAME=max`.

The command line options are lowercase, using dot as separator, prepended with `--`.
The same example as command line option: `./hercules --api.auth.username`.

Some command line options have a shortcut, which is a single letter prepended with a
single hyphen. For example `--config="path/to/config.json"` can also be specified as
`-c="path/to/config.json"`.

A parameter without a value is considered to be a boolean set to true. Example: `--light`

You can check [hercules.config.json](hercules.config.json) for a complete configuration file
with default values. You can use the rules above to override those values with environment vars
or command line options. Or write a new config file.

### Options

Below is a complete list of all the options in dot notation (as command line params).
The shortcuts, if available, are also specified. Use the rules above if you want to set those
variables in a config file or in environment.

#### --api.auth.username="xyz" --api.auth.password="abc"

Set username and password for basic http authentication when accessing the API.
Both have to be set for the authentication to work.
By default, the authentication is disabled.

#### --api.debug

Log each request that is made to the API. Default is off

#### --api.host="127.0.0.1" or -h="127.0.0.1"

The host to listen to. Setting it to `127.0.0.1` will make the API only accept localhost connections.
the default is `0.0.0.0`` which allows the API server to listen to all connections.

#### TODO

## Running on low-end devices

TODO: setting correct options and swappiness. Performance considerations (snapshots)

## API documentation

TODO: difference to the default IRI API. Snapshots, account listing.

## Pending: Roadmap

1. PoW - attachToTangle
2. Integration tests for the tangle module.
3. Integration tests for the snapshot module.

## Contributing

### Developers

Hercules can definitely be further optimized for performance and better resource management.
If you are a golang expert, feel welcome to make Hercules better!

### Testers

Run Hercules and let us know of any bugs, and suggestions you might come up with!

### Donations

**Donations always welcome**:

```
YHZIJOENEFSDMZGZA9WOGFTRXOFPVFFCDEYEFHPUGKEUAOTTMVLPSSNZNHRJD99WAVESLFPSGLMTUEIBDZRKBKXWZD
```

## Authors

* **Roman Semko** - *SemkoDev* - (https://www.twitter.com/romansemko)
* **Vitaly Semko** - *SemkoDev* - (https://www.twitter.com/dev_wit)

## License

This project is licensed under AGPLv3 - see the [LICENSE.md](LICENSE.md) file for details.