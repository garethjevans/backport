package webhook_test

import (
	"testing"

	"github.com/garethjevans/backport/pkg/webhook"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestWorkflow(t *testing.T) {
	type test struct {
		body             string
		existingBranches []string
		expectedLabels   []string
		expectedMessages []string
	}

	tests := []test{
		{
			body:             "/backport 1.1.x",
			existingBranches: []string{"1.1.x", "1.2.x"},
			expectedLabels:   []string{"Backport to 1.1.x"},
		},
		{
			body:             "/backport 1.2.x",
			existingBranches: []string{"1.1.x", "1.2.x"},
			expectedLabels:   []string{"Backport to 1.2.x"},
		},
		{
			body:             "/backport 1.1.x\n/backport 1.2.x",
			existingBranches: []string{"1.1.x", "1.2.x"},
			expectedLabels:   []string{"Backport to 1.1.x", "Backport to 1.2.x"},
		},
		{
			body:             "/backport 1.1.x\n/backport 1.2.x",
			existingBranches: []string{"1.1.x"},
			expectedLabels:   []string{"Backport to 1.1.x"},
			expectedMessages: []string{"Unable to locate branch 1.2.x"},
		},
	}

	for _, test := range tests {
		t.Run(test.body, func(t *testing.T) {
			c := webhook.Controller{}
			l := logrus.WithField("x", "y")

			err := c.HandleComment(l, "", "owner", "repo", test.body, 1)
			assert.NoError(t, err)

			labels, messages, err := webhook.DetermineLabelsToAddFromComment(test.body, &fakeLister{
				branches: test.existingBranches,
			})

			assert.NoError(t, err)
			assert.Equal(t, test.expectedLabels, labels)
			assert.Equal(t, test.expectedMessages, messages)
		})
	}
}

type fakeLister struct {
	branches []string
}

func (f *fakeLister) Branches() ([]string, error) {
	return f.branches, nil
}
