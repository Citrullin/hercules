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

2. Bundle Validator
3. PoW
4. API: including the snapshots API, password protection, duration, checks, etc.

7. Server: hostname support

8. Small devices support: config settings

## Cleanup

4. Adjust log levels. Cleanup.
5. Cleanup declarations from tangle to specific files.
6. Write comments and descriptions.
7. Check variable names for clean code.
