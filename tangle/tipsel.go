package tangle

import (
	"math"
	"math/rand"
	"bytes"

	"time"

	"../db"
	"../logs"
	"../convert"
	"../transaction"
	"github.com/dgraph-io/badger"
)

const (
	MinTipselDepth      = 2
	MaxTipselDepth      = 5
	MaxCheckDepth       = 15
	MaxTipAge           = MaxTipselDepth * time.Duration(40) * time.Second
	tipAlpha            = 0.001
	maxTipSearchRetries = 20
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

func getReference(reference []byte, depth int) []byte {
	if reference != nil && len(reference) > 0 {
		key := db.GetByteKey(reference, db.KEY_HASH)
		if db.Has(key, nil) {
			return key
		}
	}
	return GetMilestoneKeyByIndex(LatestMilestone.Index-depth, true)
}

// 2. Build sub-graph

/*
Creates a sub-graph structure, directly dropping contradictory transactions.
*/
func buildGraph(reference []byte, graphRatings *map[string]*GraphRating, seen map[string]bool, valid bool, ledgerState map[string]int64, transactions map[string]*transaction.FastTX) *GraphNode {
	approveeKeys := findApprovees(reference)
	graph := &GraphNode{reference, nil, 1, valid, nil}

	var tx *transaction.FastTX
	tKey := string(reference)
	tx, ok := transactions[tKey]
	if !ok {
		txBytes, err := db.GetBytes(db.AsKey(reference, db.KEY_BYTES), nil)
		hash, err2 := db.GetBytes(db.AsKey(reference, db.KEY_HASH), nil)
		if err != nil || err2 != nil {
			graph.Valid = false
			return graph
		}
		trits := convert.BytesToTrits(txBytes)[:8019]
		tx = transaction.TritsToFastTX(&trits, txBytes)
		tx.Hash = hash
		transactions[tKey] = tx
	}
	graph.Tx = tx

	if graph.Valid && !hasConfirmedParent(reference, MaxCheckDepth, 0, seen, transactions) {
		graph.Valid = false
	}

	for _, key := range approveeKeys {
		stringKey := string(key)
		var subGraph *GraphNode
		graphRating, ok := (*graphRatings)[stringKey]
		if ok {
			subGraph = graphRating.Graph
		} else {
			subGraph = buildGraph(key, graphRatings, seen, graph.Valid, ledgerState, transactions)
			(*graphRatings)[stringKey] = &GraphRating{0, subGraph}
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
	return graph
}

func findApprovees(key []byte) [][]byte {
	var response [][]byte
	_ = db.DB.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false
		it := txn.NewIterator(opts)
		defer it.Close()
		prefix := db.AsKey(key, db.KEY_APPROVEE)
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			response = append(response, db.AsKey(it.Item().Key()[16:], db.KEY_HASH))
		}
		return nil
	})
	return response
}

func hasConfirmedParent(reference []byte, maxDepth int, currentDepth int, seen map[string]bool, transactions map[string]*transaction.FastTX) bool {
	key := string(reference)
	answer, has := seen[key]
	if has {
		return answer
	}
	if currentDepth > maxDepth {
		return false
	}
	if db.Has(db.AsKey(reference, db.KEY_CONFIRMED), nil) {
		seen[key] = true
		return true
	}
	/*/
	timestamp, err := db.GetInt64(db.AsKey(reference, db.KEY_TIMESTAMP), nil)
	if err != nil || (timestamp > 0 && time.Now().Sub(time.Unix(timestamp, 0)) > MaxTipAge)     {
		seen[key] = false
		return false
	}
	/**/

	tKey := string(reference)
	tx, ok := transactions[tKey]
	if !ok {
		txBytes, err := db.GetBytes(db.AsKey(reference, db.KEY_BYTES), nil)
		hash, err2 := db.GetBytes(db.AsKey(reference, db.KEY_HASH), nil)
		if err != nil || err2 != nil {
			return false
		}
		trits := convert.BytesToTrits(txBytes)[:8019]
		tx = transaction.TritsToFastTX(&trits, txBytes)
		tx.Hash = hash
		transactions[tKey] = tx
	}

	if bytes.Equal(tx.TrunkTransaction, tx.BranchTransaction) {
		seen[key] = false
		return false
	}
	trunkOk := hasConfirmedParent(db.GetByteKey(tx.TrunkTransaction, db.KEY_HASH), maxDepth, currentDepth + 1, seen, transactions)
	branchOk := hasConfirmedParent(db.GetByteKey(tx.BranchTransaction, db.KEY_HASH), maxDepth, currentDepth + 1, seen, transactions)
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

func walkGraph(rating *GraphRating, ratings map[string]*GraphRating, exclude map[string]bool, ledgerState map[string]int64, transactions map[string]*transaction.FastTX) *GraphRating {
	if rating.Graph.Children == nil {
		if canBeUsed(rating, ledgerState, transactions) {
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
			graph := walkGraph(ratings[string(child.Key)], ratings, exclude, ledgerState, transactions)
			if graph != nil {
				return graph
			}
		}
	}
	if canBeUsed(rating, ledgerState, transactions) {
		return rating
	} else {
		return nil
	}
}

func canBeUsed(rating *GraphRating, ledgerState map[string]int64, transactions map[string]*transaction.FastTX) bool {
	return rating.Graph.Valid && rating.Graph.Tx.CurrentIndex == 0 && isConsistent([]*transaction.FastTX{rating.Graph.Tx}, ledgerState, transactions)
}

func isConsistent (entryPoints []*transaction.FastTX, ledgerState map[string]int64, transactions map[string]*transaction.FastTX) bool {
	ledgerDiff := make(map[string]int64)
	for _, tx := range entryPoints {
		tx, _ := transactions[string(db.GetByteKey(tx.Hash, db.KEY_HASH))]
		buildGraphDiff(ledgerDiff, tx, transactions)
	}
	var v []int64
	for _, val := range ledgerDiff {
		v = append(v, val)
	}

	for addrString, value := range ledgerDiff {
		if value < 0 {
			_, ok := ledgerState[addrString]
			if !ok {
				balance, err := db.GetInt64(db.GetAddressKey([]byte(addrString), db.KEY_BALANCE), nil)
				if err != nil {
					balance = 0
				}
				ledgerState[addrString] = balance
			}
			if ledgerState[addrString] + value < 0 {
				return false
			}
		}
	}

	return true
}

func buildGraphDiff (ledgerDiff map[string]int64, tx *transaction.FastTX, transactions map[string]*transaction.FastTX) {
	key := string(tx.Address)
	balance, ok := ledgerDiff[key]
	if !ok {
		balance = 0
	}
	balance += tx.Value
	ledgerDiff[key] = balance

	ancestor, ok := transactions[string(db.GetByteKey(tx.TrunkTransaction, db.KEY_HASH))]
	if ok {
		buildGraphDiff(ledgerDiff, ancestor, transactions)
	}
	ancestor, ok = transactions[string(db.GetByteKey(tx.BranchTransaction, db.KEY_HASH))]
	if ok {
		buildGraphDiff(ledgerDiff, ancestor, transactions)
	}
}

func GetTXToApprove(reference []byte, depth int) [][]byte {
	// Reference:
	reference = getReference(reference, depth)
	if reference == nil {
		return nil
	}

	// Graph:
	var seen = make(map[string]bool)
	var ledgerState = make(map[string]int64)
	var transactions = make(map[string]*transaction.FastTX)
	var graphRatings = make(map[string]*GraphRating)

	graph := buildGraph(reference, &graphRatings, seen, true, make(map[string]int64), transactions)
	graphRatings[string(reference)] = &GraphRating{0, graph}

	for _, rating := range graphRatings {
		seenRatings := make(map[string][]byte)
		rating.Rating = calculateRating(rating.Graph, seenRatings)
	}

	var results = []*GraphRating{}
	var exclude = make(map[string]bool)
	for x := 0; x < maxTipSearchRetries; x += 1 {
		r := walkGraph(graphRatings[string(reference)], graphRatings, exclude, ledgerState, transactions)
		if r != nil {
			exclude[string(r.Graph.Key)] = true
			if len(results) > 0 {
				var entries = []*transaction.FastTX{r.Graph.Tx}
				for _, x := range results {
					entries = append(entries, x.Graph.Tx)
				}
				if !isConsistent(entries, ledgerState, transactions) {
					continue
				}
			}
			results = append(results, r)
			if len(results) >= 2 {
				var answer [][]byte
				for _, r := range results {
					answer = append(answer, r.Graph.Tx.Hash)
					if len(answer) == 2 {
						return answer
					}
				}
			}
		} else {
		}
	}

	logs.Log.Debug("Could not get TXs to approve")
	return nil
}
