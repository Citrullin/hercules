package tangle

import (
	"math"
	"math/rand"

	"time"

	"../db"
	"../logs"
	"github.com/dgraph-io/badger"
)

const (
	MinTipselDepth      = 2
	MaxTipselDepth      = 15
	tipAlpha            = 0.001
	maxTipSearchRetries = 15
)

// 1. Get reference: either one provided or latest milestone - 15 milestones back

type GraphNode struct {
	Key      []byte
	Children []*GraphNode
	Count    int64
	Valid    bool
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
func buildGraph(reference []byte, graphRatings *map[string]*GraphRating, ledgerState map[string]int64, valid bool) *GraphNode {
	approveeKeys := findApprovees(reference)
	graph := &GraphNode{reference, nil, 1, valid}

	addr, err := db.GetBytes(db.AsKey(reference, db.KEY_ADDRESS_HASH), nil)
	if err != nil {
		graph.Valid = false
	} else {
		addrString := string(addr)

		value, err := db.GetInt64(db.AsKey(reference, db.KEY_VALUE), nil)
		if err != nil {
			graph.Valid = false
		} else {
			/**/
			ledgerStateCopy := make(map[string]int64)
			for k2, v2 := range ledgerState {
				ledgerStateCopy[k2] = v2
			}
			ledgerState = ledgerStateCopy
			/**/

			balance, ok := ledgerState[addrString]
			if !ok {
				balance, err := db.GetInt64(db.GetAddressKey(addr, db.KEY_BALANCE), nil)
				if err != nil {
					balance = 0
				}
				ledgerState[addrString] = balance
			}

			result := balance + value
			if result < 0 {
				graph.Valid = false
			}
			ledgerState[addrString] = result
		}
	}

	for _, key := range approveeKeys {
		stringKey := string(key)
		var subGraph *GraphNode
		graphRating, ok := (*graphRatings)[stringKey]
		if ok {
			subGraph = graphRating.Graph
		} else {
			subGraph = buildGraph(key, graphRatings, ledgerState, graph.Valid)
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

// 3. Walk the graph

func walkGraph(rating *GraphRating, ratings map[string]*GraphRating, exclude map[string]bool) *GraphRating {
	if rating.Graph.Children == nil {
		return rating
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
		if !child.Valid {
			continue
		}
		_, ignore := exclude[string(child.Key)]
		if ignore {
			continue
		}
		randomNumber -= weights[i]
		if randomNumber <= 0 {
			// 3. Select random child
			graph := walkGraph(ratings[string(child.Key)], ratings, exclude)
			if graph != nil {
				return graph
			}
		}
	}
	return nil
}

func GetTXToApprove(reference []byte, depth int) [][]byte {
	// Reference:
	reference = getReference(reference, depth)
	if reference == nil {
		return nil
	}

	// Graph:
	var ledgerState = make(map[string]int64)
	var graphRatings = make(map[string]*GraphRating)

	graph := buildGraph(reference, &graphRatings, ledgerState, true)
	graphRatings[string(reference)] = &GraphRating{0, graph}

	for _, rating := range graphRatings {
		seenRatings := make(map[string][]byte)
		rating.Rating = calculateRating(rating.Graph, seenRatings)
	}

	var results = make(map[string][]byte)
	var exclude = make(map[string]bool)
	for x := 0; x < maxTipSearchRetries; x += 1 {
		r := walkGraph(graphRatings[string(reference)], graphRatings, exclude)
		if r != nil {
			exclude[string(r.Graph.Key)] = true
			hash, err := db.GetBytes(db.AsKey(r.Graph.Key, db.KEY_HASH), nil)
			if err != nil {
				continue
			}
			results[string(hash)] = hash
			if len(results) >= 2 {
				var answer [][]byte
				for _, hash := range results {
					answer = append(answer, hash)
					if len(answer) == 2 {
						return answer
					}
				}
			}
		} else {
		}
	}
	logs.Log.Debug("Could not get TXs to approve!")
	return nil
}
