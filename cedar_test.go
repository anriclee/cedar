package cedar

import (
	"fmt"
	"log"
	"os"
	"testing"

	"github.com/vcaesar/tt"
)

var (
	cd *Cedar

	words = []string{
		"a", "aa", "ab", "ac", "abc", "abd",
		"abcd", "abde", "abdf", "abcdef", "abcde",
		"abcdefghijklmn", "bcd", "b", "xyz",
		"太阳系", "太阳系土星", "太阳系水星", "太阳系火星",
		"新星", "新星文明", "新星军团", "新星联邦共和国",
		"this", "this is", "this is a sentence.",
	}

	words2 = []string{
		"太阳系", "太阳系土星", "太阳系水星", "太阳系火星",
		"新星", "新星文明", "新星军团", "新星联邦共和国",
		"this", "this is", "this is a sentence.",
	}
)

func loadTestData() {
	if cd != nil {
		return
	}
	cd = New()
	// cd.Ordered = false

	// add the keys
	for i, word := range words2 {
		if err := cd.Insert([]byte(word), i); err != nil {
			panic(err)
		}
	}

	for _, word := range words {
		if err := cd.Delete([]byte(word)); err != nil {
			panic(err)
		}
	}

	for i, word := range words {
		if err := cd.Update([]byte(word), i); err != nil {
			panic(err)
		}
	}

	// delete some keys
	for i := 0; i < len(words); i += 4 {
		if err := cd.Delete([]byte(words[i])); err != nil {
			panic(err)
		}
	}
}

func check(cd *Cedar, ids []int, keys []string, values []int) {
	if len(ids) != len(keys) {
		log.Panicf("wrong prefix match: %d, %d", len(ids), len(keys))
	}

	for i, n := range ids {
		key, _ := cd.Key(n)
		val, _ := cd.Value(n)
		if string(key) != keys[i] || val != values[i] {
			log.Printf("key: %v, value: %v; val:%v, values:%v",
				string(key), keys[i], val, values[i])

			log.Panicf("wrong prefix match: %v, %v",
				string(key) != keys[i], val != values[i])
		}
	}
}

func checkConsistency(cd *Cedar) {
	for i, word := range words {
		id, err := cd.Jump([]byte(word), 0)
		if i%4 == 0 {
			if err == ErrNoPath {
				continue
			}

			_, err := cd.Value(id)
			if err == ErrNoValue {
				continue
			}
			panic("not deleted")
		}

		key, err := cd.Key(id)
		if err != nil {
			panic(err)
		}

		if string(key) != word {
			panic("key error")
		}

		value, err := cd.Value(id)
		if err != nil || value != i {
			fmt.Println(word, i, value, err)
			panic("value error")
		}
	}
}

func TestBasic(t *testing.T) {
	loadTestData()
	// check the consistency
	checkConsistency(cd)
}

func TestSaveAndLoad(t *testing.T) {
	loadTestData()

	err := cd.SaveToFile("cedar.gob", "gob")
	tt.Nil(t, err)
	defer os.Remove("cedar.gob")

	daGob := New()
	if err := daGob.LoadFromFile("cedar.gob", "gob"); err != nil {
		panic(err)
	}
	checkConsistency(daGob)

	err = cd.SaveToFile("cedar.json", "json")
	tt.Nil(t, err)
	defer os.Remove("cedar.json")

	daJson := New()
	if err := daJson.LoadFromFile("cedar.json", "json"); err != nil {
		panic(err)
	}
	checkConsistency(daJson)
}

func TestPrefixMatch(t *testing.T) {
	var (
		ids, values []int
		keys        []string
	)

	ids = cd.PrefixMatch([]byte("abcdefg"), 0)
	keys = []string{"ab", "abcd", "abcde", "abcdef"}
	values = []int{2, 6, 10, 9}
	check(cd, ids, keys, values)

	ids = cd.PrefixMatch([]byte("新星联邦共和国"), 0)
	keys = []string{"新星", "新星联邦共和国"}
	values = []int{19, 22}
	check(cd, ids, keys, values)

	ids = cd.PrefixMatch([]byte("this is a sentence."), 0)
	keys = []string{"this", "this is a sentence."}
	values = []int{23, 25}
	check(cd, ids, keys, values)
}

func TestOrder(t *testing.T) {
	c := New()
	err := c.Insert([]byte("a"), 1)
	tt.Nil(t, err)
	err = c.Insert([]byte("b"), 3)
	tt.Nil(t, err)
	err = c.Insert([]byte("d"), 6)
	tt.Nil(t, err)

	err = c.Insert([]byte("ab"), 2)
	tt.Nil(t, err)
	err = c.Insert([]byte("c"), 5)
	tt.Nil(t, err)
	err = c.Insert([]byte(""), 0)
	tt.Nil(t, err)
	err = c.Insert([]byte("bb"), 4)
	tt.Nil(t, err)

	ids := c.PrefixPredict([]byte(""), 0)
	if len(ids) != 7 {
		panic("wrong order")
	}

	for i, n := range ids {
		val, _ := c.Value(n)
		if i != val {
			panic("wrong order")
		}
	}
}

func TestPrefixPredict(t *testing.T) {
	var (
		ids    []int
		keys   []string
		values []int
	)

	ids = cd.PrefixPredict([]byte("新星"), 0)
	keys = []string{"新星", "新星军团", "新星联邦共和国"}
	values = []int{19, 21, 22}
	check(cd, ids, keys, values)

	ids = cd.PrefixPredict([]byte("太阳系"), 0)
	keys = []string{"太阳系", "太阳系水星", "太阳系火星"}
	values = []int{15, 17, 18}
	check(cd, ids, keys, values)
}
