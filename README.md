# CarrIOTA Hercules

Hercules is a lightweight alternative to IOTA's IRI. The main advantage is that it compiles
to native code and does not need Java Virtual Machine, which considerably decreases the amount
of resources needed while increasing the performance.

This way, Hercules can run on low-end devices such as Raspberry PI.

Another feature that goes beyond IRI is local snapshots that can be done automatically
or through the API for custom snapshotting strategies. Next step is to allow certain transactions,
bundles, addresses and tags to be persisted and kept beyond the snapshots.

Intrigued? Give it a try!

## Building from scratch

These instructions will get you a copy of the project up and running on your local machine.
If you have downloaded the binaries, you can skip this part.

### Prerequisites

You will need the latest [Golang](https://golang.org/dl/) (1.10.3+).

**Be aware that Ubuntu usually comes with an outdated version**. You can check what version
your go is with `go version` command.

Hercules communicates over UDP with other IRI and Hercules nodes. This means that if you are
behind a proxy or a router, you will need to setup a rule to allow connections to that port.
The default UDP port is `14600`.

# Pending: Roadmap

1. PoW - attachToTangle
2. Integration tests for the tangle module.
3. Integration tests for the snapshot module.