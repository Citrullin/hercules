package tangle

import (
	"bytes"
	"errors"
	"math"
	"math/rand"
	"sync"
	"time"

	"gitlab.com/semkodev/hercules/convert"
	"gitlab.com/semkodev/hercules/db"
	"gitlab.com/semkodev/hercules/db/coding"
	"gitlab.com/semkodev/hercules/db/ns"
	"gitlab.com/semkodev/hercules/transaction"
)

const (
	MinTipselDepth      = 3
	MaxTipselDepth      = 7
	MaxCheckDepth       = 70
	MaxTipAge           = MaxTipselDepth * time.Duration(40) * time.Second
	MaxTXAge            = time.Duration(60) * time.Second
	tipAlpha            = 0.01
	maxTipSearchRetries = 15
)

var (
	gTTALock     = &sync.Mutex{}
	txCache      = make(map[string]time.Time)
	transactions = make(map[string]*transaction.FastTX)
)

// 1. Get reference: either one provided or latest milestone - 15 milestones back

type GraphNode struct {
	Key      []byte
	Children []*GraphNode
	Count    int64
	Valid    bool
	Tx       *transaction.FastTX
}

type GraphRating struct {
	Rating int
	Graph  *GraphNode
}

func getReference(reference []byte, depth int, dbTx db.Transaction) (result []byte) {
	if reference != nil && len(reference) > 0 {
		key := ns.HashKey(reference, ns.NamespaceHash)
		if dbTx.HasKey(key) {
			return key
		}
	}
	return GetMilestoneKeyByIndex(LatestMilestone.Index-depth, true)
}

// 2. Build sub-graph

/*
Creates a sub-graph structure, directly dropping contradictory transactions.
*/
func buildGraph(reference []byte, graphRatings *map[string]*GraphRating, seen map[string]bool, valid bool, transactions map[string]*transaction.FastTX, dbTx db.Transaction) (*GraphNode, error) {
	approveeKeys, err := findApprovees(reference, dbTx)
	if err != nil {
		return nil, err
	}

	graph := &GraphNode{Key: reference, Children: nil, Count: 1, Valid: valid, Tx: nil}

	var tx *transaction.FastTX
	tKey := string(reference)
	tx, ok := transactions[tKey]
	if !ok {
		// Transaction is not in the list yet
		txBytes, err := dbTx.GetBytes(ns.Key(reference, ns.NamespaceBytes))
		hash, err2 := dbTx.GetBytes(ns.Key(reference, ns.NamespaceHash))
		if err != nil || err2 != nil {
			// Transaction was not found in the database
			graph.Valid = false
			return graph, nil
		}
		trits := convert.BytesToTrits(txBytes)[:8019]
		tx = transaction.TritsToFastTX(&trits, txBytes)
		tx.Hash = hash
		transactions[tKey] = tx
		transactions[string(hash)] = tx
	}
	ts := time.Now()
	txCache[tKey] = ts
	txCache[string(tx.Hash)] = ts
	graph.Tx = tx

	if graph.Valid && !hasConfirmedParent(reference, MaxCheckDepth, 0, seen, transactions, dbTx) {
		graph.Valid = false
	}

	for _, key := range approveeKeys {
		stringKey := string(key)
		var subGraph *GraphNode
		graphRating, ok := (*graphRatings)[stringKey]
		if ok {
			subGraph = graphRating.Graph
		} else {
			subGraph, err = buildGraph(key, graphRatings, seen, graph.Valid, transactions, dbTx)
			if err != nil {
				return nil, err
			}
			(*graphRatings)[stringKey] = &GraphRating{Rating: 0, Graph: subGraph}
		}
		if !graph.Valid && subGraph.Valid {
			subGraph.Valid = false
		}
		graph.Count += subGraph.Count
		if graph.Count < 0 {
			graph.Count = math.MaxInt64
		}
		graph.Children = append(graph.Children, subGraph)
	}
	return graph, nil
}

// Finds all the transactions that approve this transaction
func findApprovees(key []byte, dbTx db.Transaction) (approvees [][]byte, err error) {
	prefix := ns.HashKey(key, ns.NamespaceApprovee)
	err = dbTx.ForPrefix(prefix, false, func(key, _ []byte) (bool, error) {
		approvees = append(approvees, ns.Key(key[16:], ns.NamespaceHash))
		return true, nil
	})
	if err != nil {
		return nil, err
	}
	return approvees, nil
}

func hasConfirmedParent(reference []byte, maxDepth int, currentDepth int, seen map[string]bool, transactions map[string]*transaction.FastTX, dbTx db.Transaction) bool {
	key := string(reference)
	answer, has := seen[key]
	if has {
		return answer
	}
	if currentDepth > maxDepth {
		return false
	}
	if dbTx.HasKey(ns.Key(reference, ns.NamespaceConfirmed)) || dbTx.HasKey(ns.Key(reference, ns.NamespaceGTTA)) {
		seen[key] = true
		return true
	}
	/*/
	timestamp, err := db.GetInt64(ns.Key(reference, ns.NamespaceTimestamp), nil)
	if err != nil || (timestamp > 0 && time.Now().Sub(time.Unix(timestamp, 0)) > MaxTipAge)     {
		seen[key] = false
		return false
	}
	/**/

	tx, ok := transactions[key]
	if !ok {
		txBytes, err := dbTx.GetBytes(ns.Key(reference, ns.NamespaceBytes))
		hash, err2 := dbTx.GetBytes(ns.Key(reference, ns.NamespaceHash))
		if err != nil || err2 != nil {
			return false
		}
		trits := convert.BytesToTrits(txBytes)[:8019]
		tx = transaction.TritsToFastTX(&trits, txBytes)
		tx.Hash = hash
		transactions[key] = tx
		transactions[string(hash)] = tx
	}
	ts := time.Now()
	txCache[key] = ts
	txCache[string(tx.Hash)] = ts

	if tx.AttachmentTimestamp == 0 && !isMaybeMilestonePair(tx) {
		seen[key] = false
		return false
	}

	if bytes.Equal(tx.TrunkTransaction, tx.BranchTransaction) {
		seen[key] = false
		return false
	}
	trunkOk := hasConfirmedParent(ns.HashKey(tx.TrunkTransaction, ns.NamespaceHash), maxDepth, currentDepth+1, seen, transactions, dbTx)
	branchOk := hasConfirmedParent(ns.HashKey(tx.BranchTransaction, ns.NamespaceHash), maxDepth, currentDepth+1, seen, transactions, dbTx)
	ok = trunkOk && branchOk
	seen[key] = ok
	return ok
}

// 3. Calculate ratings

func calculateRating(graph *GraphNode, seenKeys map[string][]byte) int {
	rating := 1
	if graph.Children != nil {
		for _, child := range graph.Children {
			stringKey := string(child.Key)
			key, seen := seenKeys[stringKey]
			if !seen {
				seenKeys[stringKey] = key
				rating += calculateRating(child, seenKeys)
			}
		}
	}
	return rating
}

// 4. Walk the graph

func walkGraph(rating *GraphRating, ratings map[string]*GraphRating, exclude map[string]bool, ledgerState map[string]int64, transactions map[string]*transaction.FastTX, dbTx db.Transaction) *GraphRating {
	if rating.Graph.Children == nil {
		if canBeUsed(rating, ledgerState, transactions, dbTx) {
			return rating
		} else {
			return nil
		}
	}

	// 1. Get weighted ratings
	var highestRating = 0
	var weightsSum float64 = 0
	var weights []float64
	for _, child := range rating.Graph.Children {
		r := ratings[string(child.Key)].Rating
		weights = append(weights, float64(r))
		if r > highestRating {
			highestRating = r
		}
	}
	for i := range weights {
		weights[i] = math.Exp((weights[i] - float64(highestRating)) * tipAlpha)
		weightsSum += weights[i]
	}

	// 2. Make weighted choice
	rand.Seed(time.Now().UnixNano())
	randomNumber := rand.Float64() * weightsSum

	//randomNumber := float64(utils.Random(0, int(math.Floor(weightsSum))))
	for i, child := range rating.Graph.Children {
		randomNumber -= weights[i]
		if !child.Valid {
			continue
		}
		_, ignore := exclude[string(child.Key)]
		if ignore {
			continue
		}
		if randomNumber <= 0 {
			// 3. Select random child
			graph := walkGraph(ratings[string(child.Key)], ratings, exclude, ledgerState, transactions, dbTx)
			if graph != nil {
				return graph
			}
		}
	}
	if canBeUsed(rating, ledgerState, transactions, dbTx) {
		return rating
	}
	return nil
}

func canBeUsed(rating *GraphRating, ledgerState map[string]int64, transactions map[string]*transaction.FastTX, dbTx db.Transaction) bool {
	return rating.Graph.Valid && rating.Graph.Tx.CurrentIndex == 0 && (dbTx.HasKey(ns.Key(rating.Graph.Key, ns.NamespaceConfirmed)) ||
		dbTx.HasKey(ns.Key(rating.Graph.Key, ns.NamespaceGTTA)) ||
		isConsistent([]*GraphRating{rating}, ledgerState, transactions, dbTx))
}

func isConsistent(entryPoints []*GraphRating, ledgerState map[string]int64, transactions map[string]*transaction.FastTX, dbTx db.Transaction) bool {
	ledgerDiff := make(map[string]int64)
	seen := make(map[string]bool)
	for _, r := range entryPoints {
		tx, _ := transactions[string(r.Graph.Key)]
		buildGraphDiff(ledgerDiff, tx, transactions, seen)
	}
	var total int64 = 0
	for _, val := range ledgerDiff {
		total += val
	}

	if total != 0 {
		return false
	}

	for addrString, value := range ledgerDiff {
		if value < 0 {
			_, ok := ledgerState[addrString]
			if !ok {
				balance, err := coding.GetInt64(dbTx, ns.AddressKey([]byte(addrString), ns.NamespaceBalance))
				if err != nil {
					balance = 0
				}
				ledgerState[addrString] = balance
			}
			if ledgerState[addrString]+value < 0 {
				return false
			}
		}
	}

	return true
}

func buildGraphDiff(ledgerDiff map[string]int64, tx *transaction.FastTX, transactions map[string]*transaction.FastTX, seen map[string]bool) {
	cacheKey := string(tx.Hash)

	_, saw := seen[cacheKey]
	if saw {
		return
	} else {
		seen[cacheKey] = true
	}

	if tx.Value != 0 {
		key := string(tx.Address)
		balance, ok := ledgerDiff[key]
		if !ok {
			balance = 0
		}
		balance += tx.Value
		ledgerDiff[key] = balance
	}

	ancestor, ok := transactions[string(tx.TrunkTransaction)]
	if ok {
		buildGraphDiff(ledgerDiff, ancestor, transactions, seen)
	}
	ancestor, ok = transactions[string(tx.BranchTransaction)]
	if ok {
		buildGraphDiff(ledgerDiff, ancestor, transactions, seen)
	}
}

func GetTXToApprove(reference []byte, depth int) ([][]byte, error) {
	gTTALock.Lock()
	defer gTTALock.Unlock()
	defer cleanCache()

	dbTx := db.Singleton.NewTransaction(false)
	defer dbTx.Discard()

	// Reference:
	reference = getReference(reference, depth, dbTx)
	if reference == nil {
		return nil, nil
	}

	// Graph:
	seen := make(map[string]bool)
	ledgerState := make(map[string]int64)
	graphRatings := make(map[string]*GraphRating)

	graph, err := buildGraph(reference, &graphRatings, seen, true, transactions, dbTx)
	if err != nil {
		return nil, err
	}
	graphRatings[string(reference)] = &GraphRating{Rating: 0, Graph: graph}

	for _, rating := range graphRatings {
		seenRatings := make(map[string][]byte)
		rating.Rating = calculateRating(rating.Graph, seenRatings)
	}

	var results = []*GraphRating{}
	var exclude = make(map[string]bool)
	for x := 0; x < maxTipSearchRetries; x++ {
		r := walkGraph(graphRatings[string(reference)], graphRatings, exclude, ledgerState, transactions, dbTx)
		if r != nil {
			exclude[string(r.Graph.Key)] = true
			newResults := append(results, r)
			consistent := isConsistent(newResults, ledgerState, transactions, dbTx)
			if !consistent {
				continue
			}
			results = newResults
			if len(results) >= 2 {
				var answer [][]byte
				for _, r := range results {
					coding.PutInt64(db.Singleton, ns.Key(r.Graph.Key, ns.NamespaceGTTA), time.Now().Unix())
					answer = append(answer, r.Graph.Tx.Hash)
					if len(answer) == 2 {
						return answer, nil
					}
				}
			}
		}
	}

	return nil, errors.New("Could not get transactions to approve")
}

func cleanCache() {
	if len(txCache) < 5000 {
		return
	}

	ts := time.Now()
	var toDelete []string
	for key, value := range txCache {
		if ts.Sub(value) > MaxTXAge {
			toDelete = append(toDelete, key)
		}
	}
	for _, key := range toDelete {
		delete(txCache, key)
		delete(transactions, key)
	}
}
