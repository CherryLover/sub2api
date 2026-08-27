//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeEmailForAliasDedup(t *testing.T) {
	cases := []struct {
		name  string
		email string
		want  string
	}{
		{"plain", "user@example.com", "user@example.com"},
		{"uppercase and spaces", "  User@Example.COM ", "user@example.com"},
		{"plus alias stripped", "user+tag@example.com", "user@example.com"},
		{"gmail plus alias", "someone+bulk294@gmail.com", "someone@gmail.com"},
		{"gmail dots removed", "some.one@gmail.com", "someone@gmail.com"},
		{"gmail dots and plus", "s.o.m.e+x@gmail.com", "some@gmail.com"},
		{"googlemail folded to gmail", "user@googlemail.com", "user@gmail.com"},
		{"non-gmail keeps dots", "first.last@qq.com", "first.last@qq.com"},
		{"fqdn root dot dropped", "d.axis.2026@gmail.com.", "daxis2026@gmail.com"},
		{"fqdn root dot on other domain", "first.last@qq.com.", "first.last@qq.com"},
		{"leading plus keeps local part", "+alice@gmail.com", "+alice@gmail.com"},
		{"dot-only local part kept", "...@gmail.com", "...@gmail.com"},
		{"invalid keeps lowered raw", "not-an-email", "not-an-email"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, NormalizeEmailForAliasDedup(tc.email))
		})
	}
}

func TestNormalizeEmailForAliasDedupKeepsDistinctInboxes(t *testing.T) {
	// 剥离 "+后缀" 不能把同域下不同用户折叠成同一身份。
	require.NotEqual(t,
		NormalizeEmailForAliasDedup("+alice@gmail.com"),
		NormalizeEmailForAliasDedup("+bob@gmail.com"),
	)
	require.NotEqual(t,
		NormalizeEmailForAliasDedup("alice@gmail.com"),
		NormalizeEmailForAliasDedup("bob@gmail.com"),
	)
}

func TestEmailAliasDedupProbes(t *testing.T) {
	require.ElementsMatch(t,
		[]EmailAliasProbe{{Local: "someone", Domain: "gmailcom"}, {Local: "someone", Domain: "googlemailcom"}},
		EmailAliasDedupProbes("Some.One+tag@gmail.com"),
	)
	require.ElementsMatch(t,
		[]EmailAliasProbe{{Local: "daxis2026", Domain: "gmailcom"}, {Local: "daxis2026", Domain: "googlemailcom"}},
		EmailAliasDedupProbes("d.axis.2026@googlemail.com."),
	)
	require.Equal(t,
		[]EmailAliasProbe{{Local: "firstlast", Domain: "qqcom"}},
		EmailAliasDedupProbes("first.last+tag@qq.com"),
	)
	require.Nil(t, EmailAliasDedupProbes("not-an-email"))
	require.Nil(t, EmailAliasDedupProbes("...@gmail.com"))
}
