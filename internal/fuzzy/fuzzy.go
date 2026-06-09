package fuzzy

import (
	"sort"
	"strings"

	sahilfuzzy "github.com/sahilm/fuzzy"
)

type Candidate struct {
	Title    string
	Category string
	Aliases  []string
	Fields   []Field
	Value    any
}

type Field struct {
	Name   string
	Value  string
	Weight int
}

type Match struct {
	Candidate    Candidate
	Score        int
	TitleIndexes []int
	AliasIndexes map[string][]int
	FieldIndexes map[string][]int
}

type fieldKind int

const (
	fieldTitle fieldKind = iota
	fieldAlias
	fieldCategory
	fieldInitials
	fieldExtra
)

const (
	titleWeight    = 1000
	aliasWeight    = 900
	initialsWeight = 700
	categoryWeight = 250
	extraWeight    = 150
)

type field struct {
	kind   fieldKind
	name   string
	value  string
	alias  string
	weight int
}

func Filter(candidates []Candidate, query string) []Match {
	query = strings.TrimSpace(query)
	if query == "" {
		matches := make([]Match, 0, len(candidates))
		for _, candidate := range candidates {
			matches = append(matches, Match{Candidate: candidate})
		}
		return matches
	}

	tokens := strings.Fields(query)
	matches := make([]Match, 0, len(candidates))
	for _, candidate := range candidates {
		match := Match{
			Candidate:    candidate,
			AliasIndexes: map[string][]int{},
			FieldIndexes: map[string][]int{},
		}
		ok := true
		for _, token := range tokens {
			fieldMatch, found := bestFieldMatch(token, searchableFields(candidate))
			if !found {
				ok = false
				break
			}
			match.Score += fieldMatch.score
			mergeFieldMatch(&match, fieldMatch)
			mergeExtraFieldHighlights(&match, token, searchableFields(candidate))
		}
		if ok {
			match.TitleIndexes = sortedUnique(match.TitleIndexes)
			for alias, indexes := range match.AliasIndexes {
				match.AliasIndexes[alias] = sortedUnique(indexes)
			}
			for field, indexes := range match.FieldIndexes {
				match.FieldIndexes[field] = sortedUnique(indexes)
			}
			matches = append(matches, match)
		}
	}

	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].Score == matches[j].Score {
			return matches[i].Candidate.Title < matches[j].Candidate.Title
		}
		return matches[i].Score > matches[j].Score
	})
	return matches
}

type fieldMatch struct {
	field   field
	indexes []int
	score   int
}

func bestFieldMatch(token string, fields []field) (fieldMatch, bool) {
	var best fieldMatch
	found := false
	for _, field := range fields {
		matches := sahilfuzzy.Find(token, []string{field.value})
		if len(matches) == 0 {
			continue
		}
		score := matches[0].Score + field.weight*len(matches[0].MatchedIndexes)
		if !found || score > best.score {
			best = fieldMatch{
				field:   field,
				indexes: matches[0].MatchedIndexes,
				score:   score,
			}
			found = true
		}
	}
	return best, found
}

func mergeExtraFieldHighlights(match *Match, token string, fields []field) {
	for _, field := range fields {
		if field.kind != fieldExtra {
			continue
		}
		matches := sahilfuzzy.Find(token, []string{field.value})
		if len(matches) == 0 {
			continue
		}
		match.FieldIndexes[field.name] = append(match.FieldIndexes[field.name], matches[0].MatchedIndexes...)
	}
}

func searchableFields(candidate Candidate) []field {
	fields := []field{
		{kind: fieldTitle, value: candidate.Title, weight: titleWeight},
		{kind: fieldCategory, value: candidate.Category, weight: categoryWeight},
		{kind: fieldInitials, value: initials(candidate.Title), weight: initialsWeight},
	}
	for _, alias := range candidate.Aliases {
		fields = append(fields, field{kind: fieldAlias, value: alias, alias: alias, weight: aliasWeight})
	}
	for _, extra := range candidate.Fields {
		weight := extra.Weight
		if weight == 0 {
			weight = extraWeight
		}
		fields = append(fields, field{kind: fieldExtra, name: extra.Name, value: extra.Value, weight: weight})
	}
	return fields
}

func mergeFieldMatch(match *Match, fieldMatch fieldMatch) {
	switch fieldMatch.field.kind {
	case fieldTitle:
		match.TitleIndexes = append(match.TitleIndexes, fieldMatch.indexes...)
	case fieldAlias:
		match.AliasIndexes[fieldMatch.field.alias] = append(match.AliasIndexes[fieldMatch.field.alias], fieldMatch.indexes...)
	case fieldExtra:
		match.FieldIndexes[fieldMatch.field.name] = append(match.FieldIndexes[fieldMatch.field.name], fieldMatch.indexes...)
	}
}

func sortedUnique(indexes []int) []int {
	if len(indexes) == 0 {
		return nil
	}
	sort.Ints(indexes)
	result := indexes[:0]
	previous := -1
	for _, index := range indexes {
		if index == previous {
			continue
		}
		result = append(result, index)
		previous = index
	}
	return result
}

func initials(s string) string {
	words := strings.Fields(s)
	var b strings.Builder
	for _, word := range words {
		lower := strings.ToLower(word)
		if lower == "" {
			continue
		}
		b.WriteByte(lower[0])
	}
	return b.String()
}
