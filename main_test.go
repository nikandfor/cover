package main

import (
	"testing"

	"github.com/nikandfor/assert"
)

func TestMatchType(t *testing.T) {
	mm := func(p, t string) bool {
		ok, err := matchType(p, t)
		assert.NoError(t, err)

		return ok
	}

	assert.True(t, mm("wire", "github.com/nikandfor/tlog/wire.Encoder.AppendKeyString"))
	assert.True(t, mm("Encoder", "github.com/nikandfor/tlog/wire.Encoder.AppendKeyString"))
	assert.True(t, mm("AppendKeyString", "github.com/nikandfor/tlog/wire.Encoder.AppendKeyString"))
	assert.True(t, mm("wire.Encoder", "github.com/nikandfor/tlog/wire.Encoder.AppendKeyString"))
	assert.True(t, mm("Encoder.AppendKeyString", "github.com/nikandfor/tlog/wire.Encoder.AppendKeyString"))

	assert.False(t, mm("wire.", "github.com/nikandfor/tlog/wire.Encoder.AppendKeyString"))
	assert.False(t, mm(".Encoder.", "github.com/nikandfor/tlog/wire.Encoder.AppendKeyString"))

	assert.True(t, mm("Encoder.+AppendKeyString", "github.com/nikandfor/tlog/wire.Encoder.AppendKeyString"))
	assert.True(t, mm("Encoder\\.AppendKeyString", "github.com/nikandfor/tlog/wire.Encoder.AppendKeyString"))
	assert.False(t, mm("Encoder.\\+AppendKeyString", "github.com/nikandfor/tlog/wire.Encoder.AppendKeyString"))

	assert.False(t, mm("tlog", "github.com/nikandfor/tlog/wire.Encoder.AppendKeyString"))

	assert.True(t, mm("github.com/nikandfor/tlog/wire.Encoder.AppendKeyString", "github.com/nikandfor/tlog/wire.Encoder.AppendKeyString"))
	assert.False(t, mm("ithub.com/nikandfor/tlog/wire.Encoder.AppendKeyString", "github.com/nikandfor/tlog/wire.Encoder.AppendKeyString"))

	assert.False(t, mm("Encode", "github.com/nikandfor/tlog/wire.Encoder.AppendKeyString"))
	assert.False(t, mm("AppendKey", "github.com/nikandfor/tlog/wire.Encoder.AppendKeyString"))
}
