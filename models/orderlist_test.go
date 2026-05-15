package models_test

import (
	"testing"

	"github.com/aliraad79/Gun/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOrderList_Empty(t *testing.T) {
	var l models.OrderList
	assert.True(t, l.IsEmpty())
	assert.Equal(t, 0, l.Len())
	assert.Nil(t, l.Head())
}

func TestOrderList_PushBackOrder(t *testing.T) {
	var l models.OrderList
	level := &models.MatchEngineEntry{}

	n1 := l.PushBack(models.Order{ID: 1}, level)
	n2 := l.PushBack(models.Order{ID: 2}, level)
	n3 := l.PushBack(models.Order{ID: 3}, level)

	require.Equal(t, 3, l.Len())

	// FIFO: head is oldest, tail is newest
	assert.Equal(t, int64(1), l.Head().Order.ID)
	assert.Same(t, l.Head(), n1)
	assert.Equal(t, int64(2), n1.Next.Order.ID)
	assert.Equal(t, int64(3), n2.Next.Order.ID)
	assert.Nil(t, n3.Next)
	assert.Same(t, level, n1.Level)
}

func TestOrderList_RemoveHead(t *testing.T) {
	var l models.OrderList
	n1 := l.PushBack(models.Order{ID: 1}, nil)
	l.PushBack(models.Order{ID: 2}, nil)

	l.Remove(n1)

	assert.Equal(t, 1, l.Len())
	assert.Equal(t, int64(2), l.Head().Order.ID)
	assert.Nil(t, l.Head().Prev)
}

func TestOrderList_RemoveTail(t *testing.T) {
	var l models.OrderList
	l.PushBack(models.Order{ID: 1}, nil)
	n2 := l.PushBack(models.Order{ID: 2}, nil)

	l.Remove(n2)

	assert.Equal(t, 1, l.Len())
	assert.Equal(t, int64(1), l.Head().Order.ID)
	assert.Nil(t, l.Head().Next)
}

func TestOrderList_RemoveMiddle(t *testing.T) {
	var l models.OrderList
	l.PushBack(models.Order{ID: 1}, nil)
	n2 := l.PushBack(models.Order{ID: 2}, nil)
	l.PushBack(models.Order{ID: 3}, nil)

	l.Remove(n2)

	assert.Equal(t, 2, l.Len())
	assert.Equal(t, int64(1), l.Head().Order.ID)
	assert.Equal(t, int64(3), l.Head().Next.Order.ID)
	assert.Nil(t, l.Head().Next.Next)
}

func TestOrderList_RemoveOnlyElement(t *testing.T) {
	var l models.OrderList
	n := l.PushBack(models.Order{ID: 1}, nil)

	l.Remove(n)

	assert.True(t, l.IsEmpty())
	assert.Nil(t, l.Head())
}
