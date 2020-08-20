package stateparser

import (
	"fmt"
	"io"
	"regexp"
	"strings"
)

type StateReader interface {
	io.RuneReader
	State() interface{}
	RestoreState(interface{})
}

type Grammar func(StateReader) (interface{}, error)

var Escaper = strings.NewReplacer("\n", "\\n", "\t", "\\t")

type fatalError struct {
	err error
}

type TaggedMatch struct {
	Match interface{}
	Tag   string
}

func (fe fatalError) Error() string {
	return fmt.Sprintf("Fatal match error: %s", fe.err)
}

func Node(g Grammar, node func(interface{}) (interface{}, error)) Grammar {
	return func(sr StateReader) (interface{}, error) {
		m, err := g(sr)
		if err != nil {
			return nil, err
		}
		return node(m)
	}
}

func Resolve(g *Grammar) Grammar {
	return func(sr StateReader) (interface{}, error) {
		return (*g)(sr)
	}
}

func Set(set string) Grammar {
	set = Escaper.Replace(set)
	regset, _ := regexp.Compile(fmt.Sprintf("[%s]", set))
	return func(sr StateReader) (interface{}, error) {
		state := sr.State()
		r, _, err := sr.ReadRune()
		if err != nil {
			sr.RestoreState(state)
			return nil, err
		}
		s := string([]rune{r})
		if regset.MatchString(s) {
			return s, nil
		}
		sr.RestoreState(state)
		return nil, fmt.Errorf("Expected \"%s\", got %q", set, s)
	}
}

func Lit(text string) Grammar {
	rs := []rune(text)
	return func(sr StateReader) (interface{}, error) {
		state := sr.State()
		for _, r := range rs {
			rr, _, err := sr.ReadRune()
			if err != nil {
				sr.RestoreState(state)
				return nil, err
			}
			if rr != r {
				sr.RestoreState(state)
				return nil, fmt.Errorf("Expected %q, got %q", r, rr)
			}
		}
		return text, nil
	}
}

func And(gs ...Grammar) Grammar {
	return func(sr StateReader) (interface{}, error) {
		state := sr.State()
		matches := make([]interface{}, 0, len(gs))
		for _, g := range gs {
			m, err := g(sr)
			if err != nil {
				sr.RestoreState(state)
				return nil, err
			}
			if m != nil {
				matches = append(matches, m)
			}
		}
		return matches, nil
	}
}

func Or(gs ...Grammar) Grammar {
	return func(sr StateReader) (interface{}, error) {
		state := sr.State()
		errs := []error{}
		for _, g := range gs {
			m, err := g(sr)
			if err == nil {
				return m, nil
			}
			if _, isFE := err.(fatalError); isFE {
				return nil, err
			}
			errs = append(errs, err)
			sr.RestoreState(state)
		}
		return nil, fmt.Errorf("Or error, expected: (%v)", errs)
	}
}

func Mult(n, m int, g Grammar) Grammar {
	if m == 0 {
		m = int(^uint(0) >> 1)
	}
	return func(sr StateReader) (interface{}, error) {
		state := sr.State()
		ms := make([]interface{}, 0)
		for i := 0; i < m; i++ {
			match, err := g(sr)
			if err != nil {
				if _, isFE := err.(fatalError); isFE {
					return nil, err
				}
				if i < n {
					sr.RestoreState(state)
					return nil, err
				}
				return ms, nil
			}
			ms = append(ms, match)
		}
		return ms, nil
	}
}

func Optional(g Grammar) Grammar {
	return Mult(0, 1, g)
}

func Ignore(g Grammar) Grammar {
	return func(sr StateReader) (interface{}, error) {
		_, err := g(sr)
		if err != nil {
			return nil, err
		}
		return nil, nil
	}
}

func Require(gs ...Grammar) Grammar {
	g := And(gs...)
	return func(sr StateReader) (interface{}, error) {
		m, err := g(sr)
		if err != nil {
			return nil, fatalError{err}
		}
		return m, nil
	}
}

func Tag(tag string, g Grammar) Grammar {
	return func(sr StateReader) (interface{}, error) {
		m, err := g(sr)
		if err != nil {
			return nil, err
		}
		tm := TaggedMatch{
			Match: m,
			Tag:   tag,
		}
		return tm, nil
	}
}

func TagMatch(tag string, match interface{}) interface{} {
	return TaggedMatch{
		Match: match,
		Tag:   tag,
	}
}

func GetTag(m interface{}, tag string) interface{} {
	switch m := m.(type) {
	case []interface{}:
		for _, mi := range m {
			tm := GetTag(mi, tag)
			if tm != nil {
				return tm
			}
		}
		return nil
	case TaggedMatch:
		if tag == m.Tag {
			return m.Match
		}
		return GetTag(m.Match, tag)
	}
	return nil
}

func GetTags(m interface{}, tag string) []interface{} {
	switch m := m.(type) {
	case []interface{}:
		tms := []interface{}{}
		for _, mi := range m {
			tm := GetTags(mi, tag)
			if tm != nil {
				tms = append(tms, tm...)
			}
		}
		return tms
	case TaggedMatch:
		if tag == m.Tag {
			return append(GetTags(m.Match, tag), m.Match)
		}
		return GetTags(m.Match, tag)
	}
	return nil
}

func String(m interface{}) string {
	switch m := m.(type) {
	case []interface{}:
		ss := make([]string, len(m))
		for i, mi := range m {
			ss[i] = String(mi)
		}
		return strings.Join(ss, "")
	case string:
		return m
	}
	return ""
}
