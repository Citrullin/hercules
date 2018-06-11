## Development plan

* Tangle:
    * Solid milestones runner
* Snapshot loading from file: IRI and custom
    * Balances and spent info
    * All older TXs should be removed
    * Removing TXs
* Snapshotting
    
* Tests: DB and Tangle
    
* DB: small devices support configuration
* Server hostname support
* API interface to pair with IRI

## Is synchronized?

When there are no pending milestones left!
 => **what about fake milestones?**

## Snapshots

* Where to get spent info (IRI):
    * https://github.com/iotaledger/iri/blob/dev/src/main/resources/previousEpochsSpentAddresses.txt
* How are first TXs made after the snapshot?

### Conditions to make snapshots:

1. No pending milestones
2. No unknown confirmed TXs

This makes sure we have all milestones and confirmed TXs in the database.

### Procedure:

1. Lock the database.
2. Run all confirmations recursively until the queue is at zero.


# Pending

## TODO:

1. Regular auto-snapshots while running the tangle?
    - No. Since if there is no internet connection, there is no way to know when there were no TXs.
    - Alternatively one has to keep track of the incoming TXs.
    - Let it be a problem for the snapshotters

2. Snapshot API

8. Small devices support: config settings
9. PoW

## Cleanup

4. Adjust log levels. Cleanup.
5. Cleanup declarations from tangle to specific files.
8. Best way to include dependencies?

# Roadmap

2. Eventually, add PoW

To finish:
0. Co-host dependencies?
2. Write documentation (README and wiki)
3. Add License
4. Write release article.