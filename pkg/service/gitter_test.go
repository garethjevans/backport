package service

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestGitter(t *testing.T) {
	gitter := newGitter()
	t.Logf("messages: %s", gitter.messages)

	gitter.executeGit(".", "status")
	t.Logf("messages: %s", gitter.messages)

	gitter.executeGit(".", "diff")
	t.Logf("messages: %s", gitter.messages)

	assert.Equal(t, len(gitter.messages), 5)
}
