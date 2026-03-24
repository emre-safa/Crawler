package web

import (
	"sort"

	"github.com/emre-safa/crawler/internal/filestorage"
	"github.com/emre-safa/crawler/internal/index"
	"github.com/emre-safa/crawler/internal/types"
)

// relevanceScore computes: (frequency × 10) + 1000 (exact match bonus) − (depth × 5)
func relevanceScore(freq, depth int) float64 {
	return float64(freq*10) + 1000.0 - float64(depth*5)
}

// searchWords performs an AND-query across [letter].data files.
// Returns paginated results sorted by sortBy ("relevance" or "freq").
func searchWords(ws *filestorage.WordStore, query string, page, perPage int, sortBy string) ([]types.SearchResult, int) {
	tokens := index.Tokenize(query)
	if len(tokens) == 0 {
		return nil, 0
	}

	// For each token, collect entries from the appropriate [letter].data file
	// Key: URL -> aggregated result
	type urlAccum struct {
		result types.SearchResult
		freq   int
		depth  int
	}

	tokenResults := make([]map[string]*urlAccum, len(tokens))
	for i, token := range tokens {
		entries, err := ws.Lookup(token)
		if err != nil || len(entries) == 0 {
			return nil, 0 // AND semantics: if any token has no results, return empty
		}

		m := make(map[string]*urlAccum, len(entries))
		for _, e := range entries {
			if existing, ok := m[e.URL]; ok {
				existing.freq += e.Freq
			} else {
				m[e.URL] = &urlAccum{
					result: types.SearchResult{
						URL:       e.URL,
						OriginURL: e.Origin,
						Depth:     e.Depth,
					},
					freq:  e.Freq,
					depth: e.Depth,
				}
			}
		}
		tokenResults[i] = m
	}

	// Intersect: find URLs present in ALL token result sets
	smallestIdx := 0
	for i, m := range tokenResults {
		if len(m) < len(tokenResults[smallestIdx]) {
			smallestIdx = i
		}
	}

	var results []types.SearchResult
	for url, acc := range tokenResults[smallestIdx] {
		matchAll := true
		totalFreq := acc.freq
		minDepth := acc.depth
		for i, m := range tokenResults {
			if i == smallestIdx {
				continue
			}
			if other, ok := m[url]; ok {
				totalFreq += other.freq
				if other.depth < minDepth {
					minDepth = other.depth
				}
			} else {
				matchAll = false
				break
			}
		}
		if matchAll {
			r := acc.result
			r.Freq = totalFreq
			r.Depth = minDepth
			r.RelevanceScore = relevanceScore(totalFreq, minDepth)
			results = append(results, r)
		}
	}

	// Sort
	if sortBy == "relevance" {
		sort.Slice(results, func(i, j int) bool {
			return results[i].RelevanceScore > results[j].RelevanceScore
		})
	} else {
		sort.Slice(results, func(i, j int) bool {
			return results[i].Freq > results[j].Freq
		})
	}

	// Paginate
	total := len(results)
	start := (page - 1) * perPage
	if start >= total {
		return nil, total
	}
	end := start + perPage
	if end > total {
		end = total
	}
	return results[start:end], total
}
