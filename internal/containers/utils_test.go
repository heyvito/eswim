package containers

import (
	"github.com/stretchr/testify/assert"
	"math/rand"
	"testing"
)

func TestInsertAt(t *testing.T) {
	arr := []int{1, 2, 3}
	idxToInsert := rand.Intn(len(arr))
	arr = InsertAt(arr, 4, idxToInsert)
	assert.ElementsMatch(t, []int{1, 2, 3, 4}, arr)
}
