//go:build unit

package service

import (
	"bytes"
	"mime/multipart"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestValidateRequestModelCarriersJSON(t *testing.T) {
	t.Run("duplicate model", func(t *testing.T) {
		err := ValidateRequestModelCarriers("application/json", []byte(`{"model":"cheap","model":"expensive"}`), false)
		require.EqualError(t, err, "model is specified more than once")
	})
	t.Run("case variant duplicate model", func(t *testing.T) {
		err := ValidateRequestModelCarriers("application/json", []byte(`{"model":"cheap","MODEL":"expensive"}`), false)
		require.EqualError(t, err, "model is specified more than once")
	})
	t.Run("unicode escaped duplicate model", func(t *testing.T) {
		err := ValidateRequestModelCarriers("application/json", []byte(`{"model":"cheap","\u006dodel":"expensive"}`), false)
		require.EqualError(t, err, "model is specified more than once")
	})
	t.Run("nested model does not count", func(t *testing.T) {
		err := ValidateRequestModelCarriers("application/json", []byte(`{"model":"a","messages":[{"model":"b"}]}`), false)
		require.NoError(t, err)
	})
	t.Run("duplicate live session", func(t *testing.T) {
		err := ValidateRequestModelCarriers("application/json", []byte(`{"session":{"model":"a"},"session":{"model":"b"}}`), true)
		require.EqualError(t, err, "session is specified more than once")
	})
	t.Run("duplicate live session model", func(t *testing.T) {
		err := ValidateRequestModelCarriers("application/json", []byte(`{"session":{"model":"a","model":"b"}}`), true)
		require.EqualError(t, err, "session.model is specified more than once")
	})
	t.Run("invalid json stays endpoint concern", func(t *testing.T) {
		require.NoError(t, ValidateRequestModelCarriers("application/json", []byte(`{"model":`), false))
	})
}

func TestValidateRequestModelCarriersMultipart(t *testing.T) {
	build := func(fields [][2]string) (string, []byte) {
		var body bytes.Buffer
		w := multipart.NewWriter(&body)
		for _, field := range fields {
			require.NoError(t, w.WriteField(field[0], field[1]))
		}
		require.NoError(t, w.Close())
		return w.FormDataContentType(), body.Bytes()
	}
	t.Run("duplicate model", func(t *testing.T) {
		ct, body := build([][2]string{{"model", "a"}, {"model", "b"}})
		require.EqualError(t, ValidateRequestModelCarriers(ct, body, false), "model is specified more than once")
	})
	t.Run("duplicate session", func(t *testing.T) {
		ct, body := build([][2]string{{"session", `{"model":"a"}`}, {"session", `{"model":"b"}`}})
		require.EqualError(t, ValidateRequestModelCarriers(ct, body, true), "session is specified more than once")
	})
	t.Run("duplicate model inside session", func(t *testing.T) {
		ct, body := build([][2]string{{"session", `{"model":"a","model":"b"}`}})
		require.EqualError(t, ValidateRequestModelCarriers(ct, body, true), "session.model is specified more than once")
	})
	t.Run("single model is allowed", func(t *testing.T) {
		ct, body := build([][2]string{{"model", "a"}, {"prompt", "hello"}})
		require.NoError(t, ValidateRequestModelCarriers(ct, body, false))
	})
}

func TestReplaceModelInBodyCollapsesCaseVariantDuplicates(t *testing.T) {
	body := []byte(`{"alpha":1,"MODEL":"cheap","messages":[],"model":"expensive","omega":2}`)
	out := ReplaceModelInBody(body, "mapped")
	require.False(t, HasDuplicateTopLevelKey(out, "model"))
	require.Equal(t, "mapped", gjson.GetBytes(out, "model").String())
}

func TestReplaceModelInBodyCollapsesUnicodeEscapedDuplicates(t *testing.T) {
	body := []byte(`{"alpha":1,"\u006dodel":"cheap","messages":[],"MODEL":"expensive","omega":2}`)
	out := ReplaceModelInBody(body, "mapped")
	require.False(t, HasDuplicateTopLevelKey(out, "model"))
	require.Equal(t, "mapped", gjson.GetBytes(out, "model").String())
	require.Contains(t, string(out), `"model":"mapped"`)
}

func TestValidateLiveCallRequestRejectsDuplicateSessionModel(t *testing.T) {
	err := ValidateLiveCallRequest(&LiveCallRequest{
		SDP:     "v=0",
		Session: []byte(`{"model":"a","model":"b"}`),
	})
	require.EqualError(t, err, "session.model is specified more than once")
}
