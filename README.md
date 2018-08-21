# Deviota Hercules

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

Now run `go version` to make sure that everything works and you have the correct version installed. You may need to reboot (`sudo reboot`) for the go env variable changes to take effect.

### Install gcc

```
sudo apt-get install gcc
```

### Get Hercules

**Using "git clone"**

```
cd $HOME
git clone https://gitlab.com/semkodev/hercules.git
cd hercules
```

This will download and place the hercules source files in `$HOME` (/home/your_user_name).

## Building hercules

If you want to build Hercules in a different directory you just need to change the path in the commands below:

```
go get -d ./...
mkdir build
go build -v -o build/hercules hercules.go
```

That's it! Hercules is ready to be used.

## Running hercules

Running Hercules can be as easy as `./hercules`. For devices with very limited, please check
[Running on low-end devices](#running-on-low-end-devices).

Depending on your OS, you might want to [create a service](https://gitlab.com/semkodev/hercules/wikis/Creating-hercules-service-on-linux-systems) or a startup script to run hercules
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

#### --api.cors.setAllowOriginToAll

Defines if the CORS Header 'Access-Control-Allow-Origin' is set to '*'.
By default, this option is enabled. Otherwise it is not set at all.

#### --api.debug

Log each request that is made to the API. Default is off

#### --api.http.useHttp=false

Node will NOT accept API requests using HTTP. Default is on.

#### --api.http.host="127.0.0.1" or -h="127.0.0.1"

Setting it to `127.0.0.1` will make the API only accept localhost connections.
The default is `0.0.0.0` which allows the API server to listen to connections in all networks (including internet!).

#### --api.http.port=14265 or -p=14265

Port the API server should listen for HTTP requests on.

#### --api.https.useHttps

Node will accept API requests using HTTPS. Default is off.

#### --api.https.host="192.168.0.0"

Setting it to `192.168.0.0` will make the API only accept connections from that network.
The default is `0.0.0.0` which allows the API server to listen to connections in all networks (including internet!).

#### --api.https.port=14266

Port the API server should listen for HTTPS requests on.
If you want to use port 443 you need to run the command below to allow hercules to listen on that port

```
sudo setcap CAP_NET_BIND_SERVICE=+eip /path/to/hercules
```

#### --api.https.certificatePath="cert.pem"

Path to certificate file

#### --api.https.privateKeyPath="key.pem"

Path to private key used to generate the certificate file

#### --api.limitRemoteAccess="getNeighbors,addNeighbors,removeNeighbors"

Similar to IRI, limit remote execution of certain API commands.

#### --config="hercules.config.json" or -c="hercules.config.json"

Path to an configuration file in JSON format.

#### --database.path="data"

Path where the database will be stored.

#### --light

Use this flag for low-end devices with less than 2-3 GB RAM.
It makes Hercules a little slower, but more stable in hard
environments like the Raspberry Pi 3.

#### --log.hello=false

Do not show the hercules banner at the start of the logs.

#### --log.level="INFO"

Sets, what log levels should be logged. Anything above the given level
will be logged. Possible values: `CRITICAL`, `ERROR`, `WARNING`, `NOTICE`,
`INFO` and `DEBUG`. For more information, set this to DEBUG.

#### --log.useRollingLogFile

Enable save log messages to rolling log files

#### --log.logFile="hercules.log" 

Path to file where log files are saved

#### --log.maxLogFileSize=10

Maximum size in megabytes for log files. Default is 10MB

#### --log.maxLogFilesToKeep=1

Maximum amount of log files to keep when a new file is created. Default is 1 file

#### --log.criticalErrorsLogFile="herculesCriticalErrors.log"

Path to file where critical error messages are saved

#### --node.neighbors="0.0.0.0:14600,1.1.1.1:14600" or -n="0.0.0.0:14600,1.1.1.1:14600"

Static neighbors to connect to. Each neighbor consists of an IP address and an **UDP** port.

#### --node.port=14600 or -u=14600

UDP port to be used for your Hercules node.

#### --snapshots.path="snapshots"

Path where to store the snapshots.

#### --snapshots.filename=""

Filename to use for the generated snapshots. If this value is empty a dynamic name will be used,
consisting of a unix timestamp: `<timestamp>.snap`

#### --snapshots.loadFile="snapshots/1234567.snap"

When you first start Hercules, you should load an initial
snapshot, which contains spent addresses and balances. Without it,
Hercules will fail to start. Once the database is initialised, you do not
need this option any longer. It will be ignored if the database has been already initialised
with a snapshot file. The same applies for the IRI snapshots below:

#### --snapshots.loadIRIFile="snapshotMainnet.txt" --snapshots.loadIRISpentFile="previousEpochsSpentAddresses.txt" --snapshots.loadIRITimestamp=1525017600

Instead of using proprietary Hercules snapshot files, you can use the snapshot files
from the IRI that can be downloaded [here](https://github.com/iotaledger/iri/tree/dev/src/main/resources).
All three options have to be provided. Make sure you give the correct timestamp, for which
the snapshot is being loaded. You can calculate the timestamp using an [online tool](https://www.unixtimestamp.com/index.php).

The advantage is that this snapshot is official. The downside is that it will take you longer to sync.
It might be even too much for a low-end device.

As an alternative to using the official snapshot, we will provide a tool that
automatically compares the balances between a given IRI and Hercules instances to
make sure that both a in sync and have the same balances. This way you can check
any time that your synced node used a correct snapshot that has not been tampered with.

Other type of public service for snapshot consensus is coming soon.

#### --snapshots.interval=0

Interval in hours, how often do make an automated snapshot. Default is zero - disabled.

#### --snapshots.period=168

How much history of tangle to keep. Value in hours. Minimal value is 12 hours.
Lesser period increases the probability that some addresses will not be consistent with the global state.
If your node can handle more, we suggest keeping several days or a week worth of data.

#### --snapshots.keep=true

If set, the snapshots will be generated without trimming the old transactions from the database.
That is, the database is kept without modifications so that transaction history is preserved.

Keeping specific TXs by budle, address, tag, etc is coming soon.

#### --snapshots.enableapi=false

True by default. Enables additional API commands for the snapshots. More info below:

## Snapshots

Please be aware that this is an experimental feature. The minimal period is 6 hours.
We advise setting it to at least 12-24 hours on low-end devices and about a week
for normal nodes.

If the tangle is not fully synchronized, the snapshot will be skipped until the next
`snapshots.interval` time. You can start a snapshot anytime for any unix time in the past
(between now and the snapshot time currently loaded in the database) using the snapshots
API.

The snapshots are stored in the directory defined by the `snapshots.path` option.
They are currently not auto-deleted (currently about 80MB each).

If `snapshots.filename` option has been provided, it will be used as a filename for the snapshot.
Otherwise, a dynamic `<timestamp>.snap` name will be used. You have an option to set
another filename, when calling the API by providing `filename` property in your API request JSON:

```
curl http://localhost:14265/snapshots   -X POST   -H 'Content-Type: application/json'   -H 'X-IOTA-API-Version: 1'   -d '{"command": "makeSnapshot", "timestamp": 12345, "filename": "latest.snap"}'
```

If that file already exists, it will be overwritten.

### API

Make sure you disable at least `makeSnapshot` command with `api.limitRemoteAccess` or
make the API accessible for local requests only.

The snapshots API works with `POST` HTTP requests similar to the basic `IOTA` commands,
however it is placed in `/snapshots` path.

This is a preliminary description of the API. More detailed API reference will
be posted in the project wiki.

#### Getting snapshots info

```
curl http://localhost:14265/snapshots -X POST -H 'Content-Type: application/json' -H 'X-IOTA-API-Version: 1' -d '{"command": "getSnapshotsInfo"}' | jq

{
  "currentSnapshotTimestamp": 1528908287,
  "duration": 1.307782514,
  "inProgress": false,
  "isSynchronized": true,
  "snapshots": [
    {
      "checksum": "f37fdacddeee49df8f0a3a47ae808be9",
      "path": "/snapshots/1528357201.snap",
      "timestamp": 1528357201
    },
    {
      "checksum": "57d868ec86dd0f55e9f0f383d1e0f35b",
      "path": "/snapshots/1528560748.snap",
      "timestamp": 1528560748
    }
  ],
  "time": 1528991928,
  "unfinishedSnapshotTimestamp": -1
}
```

You can also request only the latest snapshot available from a node with:
``` 
curl http://localhost:14265/snapshots -X POST -H 'Content-Type: application/json' -H 'X-IOTA-API-Version: 1' -d '{"command": "getLatestSnapshotInfo"}' | jq 

{
  "currentSnapshotTimeHumanReadable": "29 Jul 18 23:03 UTC",
  "currentSnapshotTimestamp": 1532905403,
  "duration": 134,
  "inProgress": false,
  "isSynchronized": true,
  "latestSnapshot": {
    "TimeHumanReadable": "30 Jul 18 12:36 UTC",
    "checksum": "38748c56b5f55db1129918deb8d930d4",
    "path": "/snapshots/1532954172.snap",
    "timestamp": 1532954172
  },
  "time": 1533424644,
  "unfinishedSnapshotTimeHumanReadable": "",
  "unfinishedSnapshotTimestamp": -1
}
```

The timestamps are in unix timestamp format. The snapshot files are named `<unix-timestamp-in-seconds>.snap`.
The checksum for two snapshots made by two different, synchronised non-light nodes for the same timestamp
should be the same. It is worth noting that `light nodes do not sort` the addresses
in the snapshot and will therefore have different checksum.

The checksum is meant to be used later on for automatic services to compare
and download the correct snapshot for a new node from a set of public nodes that
create snapshots for specific points in time.

When there is a snapshot in progress, `inProgress` will be true and `unfinishedSnapshotTimestamp`
will be set to the snapshot time that is being generated.

If only `unfinishedSnapshotTimestamp` is set while `inProgress` is false, it can mean two things:

1. The snapshot is generated, but is currently being saved (which can take up to 30-40 minutes on a low-end device).
2. The snapshot generation failed for some reason.

#### Downloading a snapshot

Using the `path` from the response above, you can downlaod the specific snapshot:

```
wget http://localhost:14265/snapshots/1528560748.snap
```

#### Triggering a snapshot

The snapshot can be triggered with the `makeSnapshot` command.
Please make sure it is not accessible from the outside either by:

1. Running your API listening to localhost requests only.
2. Adding `makeSnapshot` to `api.limitRemoteAccess`.
3. Setting `snapshots.enableapi` to false.

To trigger a snapshot, you have to provide a unix timestamp at what
point in time the snapshot should be made. Any thing earlier than that
timestamp will be snapshotted and marked for trimming.

The trimming process is slow-ish to ensure consistency. So do not wonder
that the database does not decrease in size right after triggering or completing the snapshot.
The more transactions are to be trimmed, the longer it takes.

```
curl http://localhost:14265/snapshots   -X POST   -H 'Content-Type: application/json'   -H 'X-IOTA-API-Version: 1'   -d '{"command": "makeSnapshot", "timestamp": 1528560748 }' | jq
```

### Future development

Permanent storage of specific transactions, addresses, bundles and tags
is being prepared and will be released soon.

## Running on low-end devices

We currently officially support devices of 1GB RAM and above withat least two cores.
There are currently tests using even smaller devices, which still need time to finish.

The following setup refers to Raspberry Pi3 running Raspberian (Linux) OS.

Follow the steps described above (installing Go and Hercules). Additionally:

### Add more swap and increase swappiness

The default swap size on RPi is 100MB. Sometimes, when compacting the database
the memory usage of Hercules can spike beyond the 1GB + 100MB limit. Hence, it
is a good idea adding more swap.

RPi uses `dphys-swapfile` for seap management. Simply change `/etc/dphys-swapfile`
to include:

```
CONF_SWAPSIZE=2048
```

This will increase the swap file to 2GB. Finally, restart `dphys-swapfile`:

```
/etc/init.d/dphys-swapfile restart
```

Also, change these settings, just in case. We do not know in how far they are needed.
Our RPi's are set up like these and seem to perform nicely:

```
sysctl -w vm.overcommit_memory=1
sysctl -w vm.swappiness=75
sysctl -w vm.max_map_count=262144
```

### Run Hercules in "light" mode

If your device has less than 2-3GB in memory, you will need to add `--light` option
flag to your configuration. It makes Hercules a little slower, but much less hungry
for RAM. The **downside** is that the file storage is accessed more often than running
in full mode. On RPi, this could shorten the life of your SD Card if you use it as the
primary storage. Certainly, it depends on the quality of your SD Card.

Hence, we advise attaching a high-speed SSD external drive to store Hercules data and swap.

Our RPi's run on and off for a month now using a Samsung SD Card without any issues, yet!

## API documentation

The complete API will be described in the wiki. It is very similar to
the official IOTA API so that the official IOTA Wallet can use a Hercules node
without noticing any difference.

Biggest changes are the snapshot API described above and the `listAllAccounts` command,
which is a little CPU-intensive and simply dumps all non-zero accounts with their
respective balances. Feel free to try it out, but make sure to close it for public access.

## Pending: Roadmap

1. PoW - attachToTangle.
2. PoW - add support for PiDriver (FPGA).
3. More integration tests for the tangle module.
4. More integration tests for the snapshot module.
5. Port and integrate Nelson.

## Contributing

### Developers

Hercules can definitely be further optimized for performance and better resource management.
If you are a golang expert, feel welcome to make Hercules better!

### Testers

Run Hercules and let us know of any bugs, and suggestions you might come up with!
Please use the `issues function in gitlab. to report any problems.

### Donations

**Donations always welcome**:

```
IYUIUCFNGOEEQHT9CQU9VYJVOJMQI9VYTQGQLTBAKTFIPWWRBFEV9TJWUZU9EYEFPM9VB9QYXTSMCDKMDABASVXPPX
```

## Authors

Hercules was created and is maintained by [SemkoDev](https://semkodev.com)

- **Roman Semko** - _SemkoDev_ - (https://www.twitter.com/romansemko)
- **Vitaly Semko** - _SemkoDev_ - (https://www.twitter.com/dev_wit)

## License

This project is licensed under AGPLv3 - see the [LICENSE.md](LICENSE.md) file for details.
