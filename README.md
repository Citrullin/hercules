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

3. Why in the end there are still +400 unknown/pending TXs?
  - Maybe refs to pre-snapshot TXs? No!
  - Wrong requests to the neighbors?
  - Invalid PoW?

3. How to mark as synchronized?
  
4. Adjust log levels. Cleanup.
5. Cleanup declarations from tangle to specific files.
6. Write comments and descriptions.
7. Check variable names for clean code.