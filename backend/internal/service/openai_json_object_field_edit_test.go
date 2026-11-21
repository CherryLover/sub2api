//go:build unit

package service

import (
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// setStringFieldBySJSONLoop 是原先的写法：每处修改调用一次 sjson.SetBytes。用作参照。
func setStringFieldBySJSONLoop(t testing.TB, body []byte, arrayPath, field string, indexes []int, values []string) []byte {
	t.Helper()
	updated := body
	for k, i := range indexes {
		next, err := sjson.SetBytes(updated, arrayPath+"."+strconv.Itoa(i)+"."+field, values[k])
		require.NoError(t, err)
		updated = next
	}
	return updated
}

// assistantEdits 选出 messages 里 role=assistant 且 field 为空的对象，与两个调用方的选择方式一致。
func assistantEdits(body []byte, field string, value func(i int) string) ([]jsonObjectStringFieldEdit, []int, []string) {
	var edits []jsonObjectStringFieldEdit
	var indexes []int
	var values []string
	for i, msg := range gjson.GetBytes(body, "messages").Array() {
		if strings.TrimSpace(msg.Get("role").String()) != "assistant" || msg.Get(field).String() != "" {
			continue
		}
		v := value(i)
		edits = append(edits, jsonObjectStringFieldEdit{object: msg, value: v})
		indexes = append(indexes, i)
		values = append(values, v)
	}
	return edits, indexes, values
}

func requireSameAsSJSONLoop(t *testing.T, body []byte, field string, value func(i int) string) {
	t.Helper()
	edits, indexes, values := assistantEdits(body, field, value)
	got, ok := setStringFieldInJSONObjects(body, field, edits)
	require.True(t, ok, "single pass must handle body: %s", body)
	want := setStringFieldBySJSONLoop(t, body, "messages", field, indexes, values)
	require.Equal(t, string(want), string(got))
}

func TestSetStringFieldInJSONObjectsMatchesSJSON(t *testing.T) {
	cases := map[string]string{
		"compact": `{"model":"m","messages":[{"role":"user","content":"hi"},{"role":"assistant","content":"a"},` +
			`{"role":"assistant","content":"b","reasoning_content":""},{"role":"assistant","reasoning_content":null},` +
			`{"role":"assistant","reasoning_content":"keep"},{"role":"assistant","reasoning_content":false},{}]}`,
		"pretty": "{\n  \"messages\": [\n    { \"role\" : \"assistant\" ,\n      \"content\" : \"x\"\n    } ,\n" +
			"    {\"role\":\"assistant\"   }\t,\n    { \"role\": \"assistant\", \"reasoning_content\" :  \"\"  }\n  ],\n  \"stream\": true\n}",
		"nested field is not top-level": `{"messages":[{"role":"assistant","content":[{"type":"text","reasoning_content":"nested"}]}]}`,
		"duplicate key replaces first":  `{"messages":[{"role":"assistant","reasoning_content":"","reasoning_content":"x"}]}`,
		"unicode and escapes":           `{"messages":[{"role":"assistant","content":"中文 \"q\" \\ <"},{"role":"assistant","content":"é"}]}`,
		"no assistant":                  `{"messages":[{"role":"user","content":"hi"}]}`,
	}
	values := []string{" ", "", "plain", `quote " and \ backslash`, "line\nbreak\ttab", "中文", "<tag>&amp;"}
	for name, body := range cases {
		for _, v := range values {
			t.Run(fmt.Sprintf("%s/%q", name, v), func(t *testing.T) {
				requireSameAsSJSONLoop(t, []byte(body), "reasoning_content", func(int) string { return v })
			})
		}
	}
}

func TestSetStringFieldInJSONObjectsMatchesSJSONRandomized(t *testing.T) {
	rng := rand.New(rand.NewSource(20261117))
	ws := []string{"", " ", "\n", "\t", "  \n  "}
	pick := func(xs []string) string { return xs[rng.Intn(len(xs))] }
	states := []string{"", `"reasoning_content":""`, `"reasoning_content":null`, `"reasoning_content":"r"`, `"reasoning_content":1`}
	roles := []string{"assistant", "user", "tool", " assistant "}
	contents := []string{`"x"`, `""`, `null`, `"中文"`, `"a \"b\" \\ c"`, `[{"type":"text","text":"t"}]`}
	for n := 0; n < 300; n++ {
		var b strings.Builder
		b.WriteString("{" + pick(ws) + `"model":"m",` + pick(ws) + `"messages":` + pick(ws) + "[")
		count := rng.Intn(30)
		for i := 0; i < count; i++ {
			if i > 0 {
				b.WriteString(pick(ws) + "," + pick(ws))
			}
			members := []string{
				`"role"` + pick(ws) + ":" + pick(ws) + `"` + pick(roles) + `"`,
				`"content":` + pick(ws) + pick(contents),
			}
			if s := pick(states); s != "" {
				members = append(members, s)
			}
			rng.Shuffle(len(members), func(a, c int) { members[a], members[c] = members[c], members[a] })
			if rng.Intn(10) == 0 {
				members = nil // 空对象
			}
			b.WriteString("{" + pick(ws) + strings.Join(members, pick(ws)+","+pick(ws)) + pick(ws) + "}")
		}
		b.WriteString(pick(ws) + "]" + pick(ws) + "}")
		body := []byte(b.String())
		require.True(t, gjson.ValidBytes(body), "generated invalid JSON: %s", body)
		requireSameAsSJSONLoop(t, body, "reasoning_content", func(i int) string { return "v" + strconv.Itoa(i) + pick([]string{"", "\"", "中"}) })
	}
}

func TestSetStringFieldInJSONObjectsRejectsUnreliableEdits(t *testing.T) {
	body := []byte(`{"messages":[{"role":"assistant"},{"role":"assistant"}]}`)
	msgs := gjson.GetBytes(body, "messages").Array()

	// 不是直接取自 body 的结果（Index 未知）。
	_, ok := setStringFieldInJSONObjects(body, "f", []jsonObjectStringFieldEdit{{object: gjson.Parse(`{"role":"assistant"}`), value: "v"}})
	require.False(t, ok)
	// 顺序颠倒。
	_, ok = setStringFieldInJSONObjects(body, "f", []jsonObjectStringFieldEdit{{object: msgs[1], value: "v"}, {object: msgs[0], value: "v"}})
	require.False(t, ok)
	// 不是对象。
	_, ok = setStringFieldInJSONObjects(body, "f", []jsonObjectStringFieldEdit{{object: gjson.GetBytes(body, "messages.0.role"), value: "v"}})
	require.False(t, ok)
	// 没有修改时原样返回。
	got, ok := setStringFieldInJSONObjects(body, "f", nil)
	require.True(t, ok)
	require.Equal(t, string(body), string(got))
}

func TestNormalizeOllamaCloudChatCompletionsRequestMatchesSJSON(t *testing.T) {
	body := []byte(`{"messages":[{"role":"user","content":"q"},` +
		`{"role":"assistant","content":"a","reasoning_content":"think \"1\""},` +
		`{"role":"assistant","content":"b","reasoning_content":"think 2","reasoning":""},` +
		`{"role":"assistant","content":"c","reasoning_content":"think 3","thinking":"t"},` +
		`{"role":"assistant","content":"d"}]}`)
	got := normalizeOllamaCloudChatCompletionsRequest(body)
	want := setStringFieldBySJSONLoop(t, body, "messages", "reasoning", []int{1, 2}, []string{`think "1"`, "think 2"})
	require.Equal(t, string(want), string(got))
}
