package service_test

import (
	"testing"

	"github.com/garethjevans/backport/pkg/service"

	"github.com/stretchr/testify/assert"
)

func TestGitter(t *testing.T) {
	gitter := service.NewGitter()
	t.Logf("Messages: %s", gitter.Messages)

	_, err := gitter.ExecuteGit(".", "status")
	assert.NoError(t, err)
	t.Logf("Messages: %s", gitter.Messages)

	_, err = gitter.ExecuteGit(".", "diff")
	assert.NoError(t, err)
	t.Logf("Messages: %s", gitter.Messages)

	assert.Equal(t, len(gitter.Messages), 5)
}
